package controller

import (
	"testing"

	"k8s.io/apimachinery/pkg/api/meta"

	cachev1 "github.com/s-Himansh/kubernetes-operator/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestSetReady_ClearsDegraded(t *testing.T) {
	sc := &cachev1.ShardedCache{ObjectMeta: metav1.ObjectMeta{Name: "x"}}
	setReady(sc, 3, 3)
	if sc.Status.Phase != "Ready" {
		t.Fatalf("want Ready, got %s", sc.Status.Phase)
	}
	if !meta.IsStatusConditionFalse(sc.Status.Conditions, conditionDegraded) {
		t.Fatal("Degraded should be False on ready")
	}
	if !meta.IsStatusConditionTrue(sc.Status.Conditions, conditionReady) {
		t.Fatal("Ready should be True")
	}
}

func TestSetDegraded_SetsPhase(t *testing.T) {
	sc := &cachev1.ShardedCache{ObjectMeta: metav1.ObjectMeta{Name: "x"}}
	setDegraded(sc, "InvalidSpec", "bad")
	if sc.Status.Phase != "Degraded" {
		t.Fatalf("want Degraded, got %s", sc.Status.Phase)
	}
	if !meta.IsStatusConditionTrue(sc.Status.Conditions, conditionDegraded) {
		t.Fatal("Degraded should be True")
	}
}

func TestRecordReconcile_DoesNotPanic(t *testing.T) {
	recordReconcile("ready")
	recordReconcile("progressing")
	recordReconcile("error")
}
