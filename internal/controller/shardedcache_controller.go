package controller

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	cachev1 "github.com/s-Himansh/kubernetes-operator/api/v1"
)

const (
	finalizerName        = "cache.example.com/finalizer"
	conditionReady       = "Ready"
	conditionProgressing = "Progressing"
	conditionDegraded    = "Degraded"
)

// ShardedCacheReconciler reconciles a ShardedCache object.
type ShardedCacheReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=cache.example.com,resources=shardedcaches,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cache.example.com,resources=shardedcaches/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=cache.example.com,resources=shardedcaches/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=events,verbs=create;update;patch
// +kubebuilder:rbac:groups=policy,resources=poddisruptionbudgets,verbs=get;list;watch;create;update;patch;delete

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
			recordReconcile("error")
			return ctrl.Result{}, err
		}
		// Requeue to continue reconciliation
		return ctrl.Result{Requeue: true}, nil
	}

	// Defensive validation for clusters running without webhooks
	// (e.g. `make run --enable-webhook=false`).
	if sc.Spec.Shards < 1 || sc.Spec.Shards > 64 || sc.Spec.CacheSizePerShard < 1 || sc.Spec.Image == "" {
		msg := fmt.Sprintf("invalid spec: shards=%d cacheSizePerShard=%d image=%q (want 1<=shards<=64, cacheSize>0, image non-empty)",
			sc.Spec.Shards, sc.Spec.CacheSizePerShard, sc.Spec.Image)
		logger.Info("Invalid spec", "message", msg)
		r.event(&sc, corev1.EventTypeWarning, "InvalidSpec", msg)
		setDegraded(&sc, "InvalidSpec", msg)
		recordReconcile("error")
		if err := r.Status().Update(ctx, &sc); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Reconcile Deployment
	dep := desiredDeployment(&sc)
	if err := controllerutil.SetControllerReference(&sc, dep, r.Scheme); err != nil {
		recordReconcile("error")
		return ctrl.Result{}, err
	}

	var existingDep appsv1.Deployment
	err := r.Get(ctx, client.ObjectKey{Name: dep.Name, Namespace: dep.Namespace}, &existingDep)
	if apierrors.IsNotFound(err) {
		logger.Info("Creating Deployment", "name", dep.Name, "shards", sc.Spec.Shards)
		if err := r.Create(ctx, dep); err != nil {
			r.event(&sc, corev1.EventTypeWarning, "CreateFailed", fmt.Sprintf("Create Deployment %s failed: %v", dep.Name, err))
			recordReconcile("error")
			return ctrl.Result{}, err
		}
		r.event(&sc, corev1.EventTypeNormal, "Created", fmt.Sprintf("Created Deployment %s with %d replicas", dep.Name, sc.Spec.Shards))
		setProgressing(&sc, "DeploymentCreated", fmt.Sprintf("Created Deployment %s with %d replicas", dep.Name, sc.Spec.Shards))
		recordReconcile("progressing")
		return ctrl.Result{RequeueAfter: 5 * time.Second}, r.Status().Update(ctx, &sc)
	} else if err != nil {
		recordReconcile("error")
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
			r.event(&sc, corev1.EventTypeWarning, "UpdateFailed", fmt.Sprintf("Update Deployment %s failed: %v", existingDep.Name, err))
			recordReconcile("error")
			return ctrl.Result{}, err
		}
		r.event(&sc, corev1.EventTypeNormal, "Scaled", fmt.Sprintf("Scaled to %d shards", sc.Spec.Shards))
		setProgressing(&sc, "DeploymentScaled", fmt.Sprintf("Scaled to %d shards", sc.Spec.Shards))
		recordReconcile("progressing")
		return ctrl.Result{RequeueAfter: 5 * time.Second}, r.Status().Update(ctx, &sc)
	}

	// Reconcile Service (headless). ClusterIP is immutable: recreate on drift.
	svc := desiredService(&sc)
	if err := controllerutil.SetControllerReference(&sc, svc, r.Scheme); err != nil {
		recordReconcile("error")
		return ctrl.Result{}, err
	}
	var existingSvc corev1.Service
	if err := r.Get(ctx, client.ObjectKey{Name: svc.Name, Namespace: svc.Namespace}, &existingSvc); apierrors.IsNotFound(err) {
		logger.Info("Creating Service", "name", svc.Name)
		if err := r.Create(ctx, svc); err != nil {
			r.event(&sc, corev1.EventTypeWarning, "CreateFailed", fmt.Sprintf("Create Service %s failed: %v", svc.Name, err))
			recordReconcile("error")
			return ctrl.Result{}, err
		}
		r.event(&sc, corev1.EventTypeNormal, "Created", fmt.Sprintf("Created Service %s", svc.Name))
	} else if err != nil {
		recordReconcile("error")
		return ctrl.Result{}, err
	} else if existingSvc.Spec.ClusterIP != "" && existingSvc.Spec.ClusterIP != "None" {
		logger.Info("Recreating Service as headless", "name", existingSvc.Name, "clusterIP", existingSvc.Spec.ClusterIP)
		if err := r.Delete(ctx, &existingSvc); err != nil {
			recordReconcile("error")
			return ctrl.Result{}, err
		}
		r.event(&sc, corev1.EventTypeNormal, "Recreated", fmt.Sprintf("Recreated Service %s as headless", svc.Name))
		recordReconcile("progressing")
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	// Reconcile PDB (protects rolling availability, minAvailable=1 when shards>1).
	pdb := desiredPDB(&sc)
	if err := controllerutil.SetControllerReference(&sc, pdb, r.Scheme); err != nil {
		recordReconcile("error")
		return ctrl.Result{}, err
	}
	var existingPDB policyv1.PodDisruptionBudget
	if err := r.Get(ctx, client.ObjectKey{Name: pdb.Name, Namespace: pdb.Namespace}, &existingPDB); apierrors.IsNotFound(err) {
		if err := r.Create(ctx, pdb); err != nil {
			r.event(&sc, corev1.EventTypeWarning, "CreateFailed", fmt.Sprintf("Create PDB %s failed: %v", pdb.Name, err))
			recordReconcile("error")
			return ctrl.Result{}, err
		}
	} else if err != nil {
		recordReconcile("error")
		return ctrl.Result{}, err
	} else if existingPDB.Spec.MinAvailable.String() != pdb.Spec.MinAvailable.String() {
		existingPDB.Spec.MinAvailable = pdb.Spec.MinAvailable
		if err := r.Update(ctx, &existingPDB); err != nil {
			recordReconcile("error")
			return ctrl.Result{}, err
		}
	}

	// Update status based on Deployment availability
	ready := existingDep.Status.ReadyReplicas
	sc.Status.ReadyShards = ready
	sc.Status.ObservedGeneration = sc.Generation

	if ready == sc.Spec.Shards && existingDep.Status.AvailableReplicas == sc.Spec.Shards {
		setReady(&sc, ready, sc.Spec.Shards)
	} else {
		setWaiting(&sc, ready, sc.Spec.Shards)
	}

	if err := r.Status().Update(ctx, &sc); err != nil {
		recordReconcile("error")
		return ctrl.Result{}, err
	}

	// Requeue if not yet ready
	if sc.Status.Phase != "Ready" {
		recordReconcile("progressing")
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}
	recordReconcile("ready")
	return ctrl.Result{}, nil
}

func (r *ShardedCacheReconciler) event(sc *cachev1.ShardedCache, eventType, reason, msg string) {
	if r.Recorder != nil {
		r.Recorder.Event(sc, eventType, reason, msg)
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *ShardedCacheReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&cachev1.ShardedCache{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&policyv1.PodDisruptionBudget{}).
		Complete(r)
}
