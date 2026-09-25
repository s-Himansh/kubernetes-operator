package controller

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	cachev1 "github.com/s-Himansh/kubernetes-operator/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func testShardedCache() *cachev1.ShardedCache {
	return &cachev1.ShardedCache{
		ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: "default"},
		Spec: cachev1.ShardedCacheSpec{
			Shards:            3,
			CacheSizePerShard: 100,
			Image:             "ghcr.io/s-himansh/shared-lru-cache:latest",
		},
	}
}

func TestSyncDeploymentContainers_NoDrift(t *testing.T) {
	sc := testShardedCache()
	desired := desiredDeployment(sc)
	existing := desiredDeployment(sc)

	if syncDeploymentContainers(existing, desired, sc) {
		t.Fatal("expected no mutation when specs match")
	}
}

func TestSyncDeploymentContainers_ImageDrift(t *testing.T) {
	sc := testShardedCache()
	desired := desiredDeployment(sc)
	existing := desiredDeployment(sc)
	existing.Spec.Template.Spec.Containers[0].Image = "old:1.0"

	if !syncDeploymentContainers(existing, desired, sc) {
		t.Fatal("expected mutation on image drift")
	}
	if got := existing.Spec.Template.Spec.Containers[0].Image; got != sc.Spec.Image {
		t.Fatalf("image not synced, got %s", got)
	}
	// Second pass must be stable (idempotent).
	if syncDeploymentContainers(existing, desired, sc) {
		t.Fatal("second sync should be a no-op")
	}
}

func TestSyncDeploymentContainers_ResourcesAndEnv(t *testing.T) {
	sc := testShardedCache()
	sc.Spec.Resources = corev1.ResourceRequirements{
		Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m")},
	}
	desired := desiredDeployment(sc)
	existing := desiredDeployment(testShardedCache()) // old resources + old env
	existing.Spec.Template.Spec.Containers[0].Env = nil

	if !syncDeploymentContainers(existing, desired, sc) {
		t.Fatal("expected mutation on resources/env drift")
	}
	env := map[string]string{}
	for _, e := range existing.Spec.Template.Spec.Containers[0].Env {
		env[e.Name] = e.Value
	}
	if env["CACHE_SHARDS"] != "3" || env["CACHE_CAPACITY"] != "300" {
		t.Fatalf("env not synced, got %v", env)
	}
	if syncDeploymentContainers(existing, desired, sc) {
		t.Fatal("second sync should be a no-op")
	}
}

func TestSyncEnvVars_AddsMissing(t *testing.T) {
	c := &corev1.Container{}
	if !syncEnvVars(c, map[string]string{"CACHE_SHARDS": "8"}) {
		t.Fatal("expected mutation when env missing")
	}
	if syncEnvVars(c, map[string]string{"CACHE_SHARDS": "8"}) {
		t.Fatal("repeat with same value should be a no-op")
	}
}

func TestDesiredDeployment_SecureAndHeadless(t *testing.T) {
	sc := testShardedCache()
	dep := desiredDeployment(sc)
	if dep.Spec.Template.Spec.SecurityContext == nil || dep.Spec.Template.Spec.SecurityContext.RunAsNonRoot == nil ||
		!*dep.Spec.Template.Spec.SecurityContext.RunAsNonRoot {
		t.Fatal("pod must run as non-root")
	}
	c := dep.Spec.Template.Spec.Containers[0]
	if c.SecurityContext == nil || c.SecurityContext.AllowPrivilegeEscalation == nil ||
		*c.SecurityContext.AllowPrivilegeEscalation {
		t.Fatal("container must not allow privilege escalation")
	}
	svc := desiredService(sc)
	if svc.Spec.ClusterIP != "None" {
		t.Fatalf("service must be headless, got %q", svc.Spec.ClusterIP)
	}
}

func TestDesiredPDB_MinAvailable(t *testing.T) {
	sc := testShardedCache()
	pdb := desiredPDB(sc)
	if pdb.Spec.MinAvailable.String() != "1" {
		t.Fatalf("want minAvailable 1 for shards>1, got %s", pdb.Spec.MinAvailable.String())
	}
	sc.Spec.Shards = 1
	single := desiredPDB(sc)
	if single.Spec.MinAvailable.String() != "0" {
		t.Fatalf("want minAvailable 0 for single shard, got %s", single.Spec.MinAvailable.String())
	}
}

func TestSyncPodSecurity_Enforces(t *testing.T) {
	sc := testShardedCache()
	desired := desiredDeployment(sc)
	existing := desiredDeployment(sc)
	existing.Spec.Template.Spec.SecurityContext = nil
	existing.Spec.Template.Spec.Containers[0].SecurityContext = nil
	if !syncDeploymentContainers(existing, desired, sc) {
		t.Fatal("expected mutation to enforce security contexts")
	}
	if syncDeploymentContainers(existing, desired, sc) {
		t.Fatal("second sync should be a no-op")
	}
}
