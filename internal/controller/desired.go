package controller

import (
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	cachev1 "github.com/s-Himansh/kubernetes-operator/api/v1"
)

func cacheLabels(name string) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":     "sharded-cache",
		"app.kubernetes.io/instance": name,
	}
}

func desiredDeployment(sc *cachev1.ShardedCache) *appsv1.Deployment {
	labels := cacheLabels(sc.Name)
	labels["cache.example.com/shard"] = "true"
	replicas := sc.Spec.Shards
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sc.Name + "-cache",
			Namespace: sc.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: boolPtr(true),
						SeccompProfile: &corev1.SeccompProfile{
							Type: corev1.SeccompProfileTypeRuntimeDefault,
						},
					},
					Containers: []corev1.Container{{
						Name:  "cache",
						Image: sc.Spec.Image,
						Ports: []corev1.ContainerPort{{ContainerPort: 8080, Name: "http"}},
						Env: []corev1.EnvVar{
							{Name: "CACHE_SHARDS", Value: fmt.Sprintf("%d", sc.Spec.Shards)},
							{Name: "CACHE_CAPACITY", Value: fmt.Sprintf("%d", sc.Spec.CacheSizePerShard*sc.Spec.Shards)},
							{Name: "PORT", Value: "8080"},
						},
						Resources: sc.Spec.Resources,
						SecurityContext: &corev1.SecurityContext{
							AllowPrivilegeEscalation: boolPtr(false),
							RunAsNonRoot:             boolPtr(true),
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{"ALL"},
							},
						},
						LivenessProbe: &corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								HTTPGet: &corev1.HTTPGetAction{Path: "/api/health", Port: intstr.FromInt(8080)},
							},
							InitialDelaySeconds: 5,
							PeriodSeconds:       10,
						},
						ReadinessProbe: &corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								HTTPGet: &corev1.HTTPGetAction{Path: "/api/health", Port: intstr.FromInt(8080)},
							},
							InitialDelaySeconds: 3,
							PeriodSeconds:       5,
						},
					}},
				},
			},
		},
	}
}

func desiredService(sc *cachev1.ShardedCache) *corev1.Service {
	labels := cacheLabels(sc.Name)
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sc.Name + "-cache",
			Namespace: sc.Namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			ClusterIP: "None",
			Selector:  labels,
			Ports: []corev1.ServicePort{{
				Name:       "http",
				Port:       80,
				TargetPort: intstr.FromInt(8080),
				Protocol:   corev1.ProtocolTCP,
			}},
		},
	}
}

func boolPtr(b bool) *bool { return &b }

func desiredPDB(sc *cachev1.ShardedCache) *policyv1.PodDisruptionBudget {
	labels := cacheLabels(sc.Name)
	minAvailable := intstr.FromInt32(1)
	if sc.Spec.Shards <= 1 {
		minAvailable = intstr.FromInt32(0)
	}
	return &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sc.Name + "-cache",
			Namespace: sc.Namespace,
			Labels:    labels,
		},
		Spec: policyv1.PodDisruptionBudgetSpec{
			MinAvailable: &minAvailable,
			Selector:     &metav1.LabelSelector{MatchLabels: labels},
		},
	}
}

// syncDeploymentContainers copies managed fields from desired into existing.
// It returns true if existing was mutated and needs a write.
func syncDeploymentContainers(existing *appsv1.Deployment, desired *appsv1.Deployment, sc *cachev1.ShardedCache) bool {
	mutated := false
	if syncPodSecurity(existing, desired) {
		mutated = true
	}
	if len(existing.Spec.Template.Spec.Containers) == 0 || len(desired.Spec.Template.Spec.Containers) == 0 {
		return mutated
	}

	existingContainer := &existing.Spec.Template.Spec.Containers[0]
	desiredContainer := desired.Spec.Template.Spec.Containers[0]

	if existingContainer.Image != desiredContainer.Image {
		existingContainer.Image = desiredContainer.Image
		mutated = true
	}
	if !apiequality.Semantic.DeepEqual(existingContainer.Resources, sc.Spec.Resources) {
		existingContainer.Resources = sc.Spec.Resources
		mutated = true
	}
	if syncEnvVars(existingContainer, map[string]string{
		"CACHE_SHARDS":   fmt.Sprintf("%d", sc.Spec.Shards),
		"CACHE_CAPACITY": fmt.Sprintf("%d", sc.Spec.CacheSizePerShard*sc.Spec.Shards),
	}) {
		mutated = true
	}
	return mutated
}

func syncPodSecurity(existing *appsv1.Deployment, desired *appsv1.Deployment) bool {
	mutated := false
	if !apiequality.Semantic.DeepEqual(existing.Spec.Template.Spec.SecurityContext, desired.Spec.Template.Spec.SecurityContext) {
		existing.Spec.Template.Spec.SecurityContext = desired.Spec.Template.Spec.SecurityContext
		mutated = true
	}
	if len(existing.Spec.Template.Spec.Containers) > 0 && len(desired.Spec.Template.Spec.Containers) > 0 {
		if !apiequality.Semantic.DeepEqual(existing.Spec.Template.Spec.Containers[0].SecurityContext, desired.Spec.Template.Spec.Containers[0].SecurityContext) {
			existing.Spec.Template.Spec.Containers[0].SecurityContext = desired.Spec.Template.Spec.Containers[0].SecurityContext
			mutated = true
		}
	}
	return mutated
}

func syncEnvVars(container *corev1.Container, wanted map[string]string) bool {
	mutated := false
	for k, v := range wanted {
		found := false
		for i := range container.Env {
			if container.Env[i].Name == k {
				found = true
				if container.Env[i].Value != v {
					container.Env[i].Value = v
					mutated = true
				}
				break
			}
		}
		if !found {
			container.Env = append(container.Env, corev1.EnvVar{Name: k, Value: v})
			mutated = true
		}
	}
	return mutated
}
