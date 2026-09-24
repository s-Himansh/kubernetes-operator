package v1

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var shardedcachelog = logf.Log.WithName("shardedcache-resource")

// SetupWebhookWithManager registers webhooks.
func (r *ShardedCache) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-cache-example-com-v1-shardedcache,mutating=true,failurePolicy=fail,sideEffects=None,groups=cache.example.com,resources=shardedcaches,verbs=create;update,versions=v1,name=mshardedcache.kb.io,admissionReviewVersions=v1

var _ webhook.Defaulter = &ShardedCache{}

// Default implements webhook.Defaulter so a webhook will be registered for the type.
func (r *ShardedCache) Default() {
	shardedcachelog.Info("default", "name", r.Name)
	if r.Spec.Shards == 0 {
		r.Spec.Shards = 16
	}
	if r.Spec.CacheSizePerShard == 0 {
		r.Spec.CacheSizePerShard = 1000
	}
	if r.Spec.Image == "" {
		r.Spec.Image = "ghcr.io/s-himansh/shared-lru-cache:latest"
	}
}

// +kubebuilder:webhook:path=/validate-cache-example-com-v1-shardedcache,mutating=false,failurePolicy=fail,sideEffects=None,groups=cache.example.com,resources=shardedcaches,verbs=create;update,versions=v1,name=vshardedcache.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &ShardedCache{}

// ValidateCreate implements webhook.Validator.
func (r *ShardedCache) ValidateCreate() (admission.Warnings, error) {
	return nil, r.validate()
}

// ValidateUpdate implements webhook.Validator.
func (r *ShardedCache) ValidateUpdate(_ runtime.Object) (admission.Warnings, error) {
	return nil, r.validate()
}

// ValidateDelete implements webhook.Validator.
func (r *ShardedCache) ValidateDelete() (admission.Warnings, error) {
	return nil, nil
}

func (r *ShardedCache) validate() error {
	if r.Spec.Shards < 1 || r.Spec.Shards > 64 {
		return fmt.Errorf("spec.shards must be between 1 and 64, got %d", r.Spec.Shards)
	}
	if r.Spec.CacheSizePerShard < 1 {
		return fmt.Errorf("spec.cacheSizePerShard must be > 0, got %d", r.Spec.CacheSizePerShard)
	}
	if r.Spec.Image == "" {
		return fmt.Errorf("spec.image must not be empty")
	}
	return nil
}
