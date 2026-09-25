package controller

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	cachev1 "github.com/s-Himansh/kubernetes-operator/api/v1"
)

const (
	finalizerName = "cache.example.com/finalizer"
	conditionReady       = "Ready"
	conditionProgressing = "Progressing"
)

// ShardedCacheReconciler reconciles a ShardedCache object.
type ShardedCacheReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=cache.example.com,resources=shardedcaches,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cache.example.com,resources=shardedcaches/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=cache.example.com,resources=shardedcaches/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

func (r *ShardedCacheReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var sc cachev1.ShardedCache
	if err := r.Get(ctx, req.NamespacedName, &sc); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Handle deletion with finalizer
	if !sc.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(&sc, finalizerName) {
			// Owned Deployment/Service will be GC'd via ownerReference; just remove finalizer
			controllerutil.RemoveFinalizer(&sc, finalizerName)
			if err := r.Update(ctx, &sc); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(&sc, finalizerName) {
		controllerutil.AddFinalizer(&sc, finalizerName)
		if err := r.Update(ctx, &sc); err != nil {
			return ctrl.Result{}, err
		}
		// Requeue to continue reconciliation
		return ctrl.Result{Requeue: true}, nil
	}

	// Reconcile Deployment
	dep := desiredDeployment(&sc)
	if err := controllerutil.SetControllerReference(&sc, dep, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}

	var existingDep appsv1.Deployment
	err := r.Get(ctx, client.ObjectKey{Name: dep.Name, Namespace: dep.Namespace}, &existingDep)
	if apierrors.IsNotFound(err) {
		logger.Info("Creating Deployment", "name", dep.Name, "shards", sc.Spec.Shards)
		if err := r.Create(ctx, dep); err != nil {
			return ctrl.Result{}, err
		}
		setProgressing(&sc, "DeploymentCreated", fmt.Sprintf("Created Deployment %s with %d replicas", dep.Name, *dep.Spec.Replicas))
		return ctrl.Result{RequeueAfter: 5 * time.Second}, r.Status().Update(ctx, &sc)
	} else if err != nil {
		return ctrl.Result{}, err
	}

	// Scale / update if spec drifted. Only write when managed fields differ
	// to avoid hot update loops.
	needsUpdate := false
	if existingDep.Spec.Replicas == nil || *existingDep.Spec.Replicas != sc.Spec.Shards {
		var from int32
		if existingDep.Spec.Replicas != nil {
			from = *existingDep.Spec.Replicas
		}
		logger.Info("Scaling Deployment", "from", from, "to", sc.Spec.Shards)
		existingDep.Spec.Replicas = &sc.Spec.Shards
		needsUpdate = true
	}
	if syncDeploymentContainers(&existingDep, dep, &sc) {
		needsUpdate = true
	}
	if needsUpdate {
		if err := r.Update(ctx, &existingDep); err != nil {
			return ctrl.Result{}, err
		}
		setProgressing(&sc, "DeploymentScaled", fmt.Sprintf("Scaled to %d shards", sc.Spec.Shards))
		return ctrl.Result{RequeueAfter: 5 * time.Second}, r.Status().Update(ctx, &sc)
	}

	// Reconcile Service
	svc := desiredService(&sc)
	if err := controllerutil.SetControllerReference(&sc, svc, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}
	var existingSvc corev1.Service
	if err := r.Get(ctx, client.ObjectKey{Name: svc.Name, Namespace: svc.Namespace}, &existingSvc); apierrors.IsNotFound(err) {
		logger.Info("Creating Service", "name", svc.Name)
		if err := r.Create(ctx, svc); err != nil {
			return ctrl.Result{}, err
		}
	} else if err != nil {
		return ctrl.Result{}, err
	}

	// Update status based on Deployment availability
	ready := existingDep.Status.ReadyReplicas
	sc.Status.ReadyShards = ready
	sc.Status.ObservedGeneration = sc.Generation

	if ready == sc.Spec.Shards && existingDep.Status.AvailableReplicas == sc.Spec.Shards {
		meta.SetStatusCondition(&sc.Status.Conditions, metav1.Condition{
			Type:               conditionReady,
			Status:             metav1.ConditionTrue,
			Reason:             "AllShardsReady",
			Message:            fmt.Sprintf("%d/%d shards ready", ready, sc.Spec.Shards),
			ObservedGeneration: sc.Generation,
		})
		meta.SetStatusCondition(&sc.Status.Conditions, metav1.Condition{
			Type:               conditionProgressing,
			Status:             metav1.ConditionFalse,
			Reason:             "Reconciled",
			Message:            "Desired state reached",
			ObservedGeneration: sc.Generation,
		})
		sc.Status.Phase = "Ready"
	} else {
		meta.SetStatusCondition(&sc.Status.Conditions, metav1.Condition{
			Type:               conditionProgressing,
			Status:             metav1.ConditionTrue,
			Reason:             "Scaling",
			Message:            fmt.Sprintf("Waiting for %d shards, %d ready", sc.Spec.Shards, ready),
			ObservedGeneration: sc.Generation,
		})
		meta.SetStatusCondition(&sc.Status.Conditions, metav1.Condition{
			Type:               conditionReady,
			Status:             metav1.ConditionFalse,
			Reason:             "NotReady",
			Message:            fmt.Sprintf("%d/%d shards ready", ready, sc.Spec.Shards),
			ObservedGeneration: sc.Generation,
		})
		sc.Status.Phase = "Progressing"
	}

	if err := r.Status().Update(ctx, &sc); err != nil {
		return ctrl.Result{}, err
	}

	// Requeue if not yet ready
	if sc.Status.Phase != "Ready" {
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}
	return ctrl.Result{}, nil
}

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

// SetupWithManager sets up the controller with the Manager.
func (r *ShardedCacheReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&cachev1.ShardedCache{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Complete(r)
}
