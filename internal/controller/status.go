package controller

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	cachev1 "github.com/s-Himansh/kubernetes-operator/api/v1"
)

func setProgressing(sc *cachev1.ShardedCache, reason, msg string) {
	meta.SetStatusCondition(&sc.Status.Conditions, metav1.Condition{
		Type:               conditionProgressing,
		Status:             metav1.ConditionTrue,
		Reason:             reason,
		Message:            msg,
		ObservedGeneration: sc.Generation,
	})
	sc.Status.Phase = "Progressing"
}

func setReady(sc *cachev1.ShardedCache, ready, want int32) {
	meta.SetStatusCondition(&sc.Status.Conditions, metav1.Condition{
		Type:               conditionReady,
		Status:             metav1.ConditionTrue,
		Reason:             "AllShardsReady",
		Message:            fmt.Sprintf("%d/%d shards ready", ready, want),
		ObservedGeneration: sc.Generation,
	})
	meta.SetStatusCondition(&sc.Status.Conditions, metav1.Condition{
		Type:               conditionProgressing,
		Status:             metav1.ConditionFalse,
		Reason:             "Reconciled",
		Message:            "Desired state reached",
		ObservedGeneration: sc.Generation,
	})
	meta.SetStatusCondition(&sc.Status.Conditions, metav1.Condition{
		Type:               conditionDegraded,
		Status:             metav1.ConditionFalse,
		Reason:             "Healthy",
		Message:            "Reconciled without errors",
		ObservedGeneration: sc.Generation,
	})
	sc.Status.Phase = "Ready"
}

func setWaiting(sc *cachev1.ShardedCache, ready, want int32) {
	meta.SetStatusCondition(&sc.Status.Conditions, metav1.Condition{
		Type:               conditionProgressing,
		Status:             metav1.ConditionTrue,
		Reason:             "Scaling",
		Message:            fmt.Sprintf("Waiting for %d shards, %d ready", want, ready),
		ObservedGeneration: sc.Generation,
	})
	meta.SetStatusCondition(&sc.Status.Conditions, metav1.Condition{
		Type:               conditionReady,
		Status:             metav1.ConditionFalse,
		Reason:             "NotReady",
		Message:            fmt.Sprintf("%d/%d shards ready", ready, want),
		ObservedGeneration: sc.Generation,
	})
	meta.SetStatusCondition(&sc.Status.Conditions, metav1.Condition{
		Type:               conditionDegraded,
		Status:             metav1.ConditionFalse,
		Reason:             "Healthy",
		Message:            "Progressing toward desired state",
		ObservedGeneration: sc.Generation,
	})
	sc.Status.Phase = "Progressing"
}

func setDegraded(sc *cachev1.ShardedCache, reason, msg string) {
	meta.SetStatusCondition(&sc.Status.Conditions, metav1.Condition{
		Type:               conditionDegraded,
		Status:             metav1.ConditionTrue,
		Reason:             reason,
		Message:            msg,
		ObservedGeneration: sc.Generation,
	})
	meta.SetStatusCondition(&sc.Status.Conditions, metav1.Condition{
		Type:               conditionReady,
		Status:             metav1.ConditionFalse,
		Reason:             reason,
		Message:            msg,
		ObservedGeneration: sc.Generation,
	})
	sc.Status.Phase = "Degraded"
}
