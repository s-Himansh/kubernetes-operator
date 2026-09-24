package v1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ShardedCacheSpec defines the desired state of ShardedCache.
type ShardedCacheSpec struct {
	// Shards is the desired number of cache shards / deployment replicas.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=64
	// +kubebuilder:default=16
	Shards int32 `json:"shards,omitempty"`

	// CacheSizePerShard is entries per shard.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=1000
	CacheSizePerShard int32 `json:"cacheSizePerShard,omitempty"`

	// Image is the container image for the cache pods.
	// +kubebuilder:default="ghcr.io/s-himansh/shared-lru-cache:latest"
	Image string `json:"image,omitempty"`

	// Version is an optional version label propagated to pods.
	// +optional
	Version string `json:"version,omitempty"`

	// Resources defines compute resources for each pod.
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`
}

// ShardedCacheStatus defines the observed state of ShardedCache.
type ShardedCacheStatus struct {
	// ReadyShards is number of ready replicas observed.
	// +optional
	ReadyShards int32 `json:"readyShards,omitempty"`

	// Phase is a high-level summary: Pending, Progressing, Ready, Degraded.
	// +optional
	Phase string `json:"phase,omitempty"`

	// ObservedGeneration is the last generation reconciled.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions represent latest available observations.
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=sc
// +kubebuilder:printcolumn:name="Shards",type=integer,JSONPath=`.spec.shards`
// +kubebuilder:printcolumn:name="Ready",type=integer,JSONPath=`.status.readyShards`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ShardedCache is the Schema for the shardedcaches API.
type ShardedCache struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ShardedCacheSpec   `json:"spec,omitempty"`
	Status ShardedCacheStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ShardedCacheList contains a list of ShardedCache.
type ShardedCacheList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ShardedCache `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ShardedCache{}, &ShardedCacheList{})
}
