package v1

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestShardedCacheValidate(t *testing.T) {
	valid := &ShardedCache{
		ObjectMeta: metav1.ObjectMeta{Name: "valid"},
		Spec: ShardedCacheSpec{
			Shards:            8,
			CacheSizePerShard: 100,
			Image:             "ghcr.io/s-himansh/shared-lru-cache:latest",
		},
	}
	if _, err := valid.ValidateCreate(); err != nil {
		t.Fatalf("valid spec rejected: %v", err)
	}

	cases := map[string]ShardedCacheSpec{
		"shards zero":     {Shards: 0, CacheSizePerShard: 100, Image: "img"},
		"shards too many": {Shards: 65, CacheSizePerShard: 100, Image: "img"},
		"cache size zero": {Shards: 4, CacheSizePerShard: 0, Image: "img"},
		"empty image":     {Shards: 4, CacheSizePerShard: 100, Image: ""},
	}
	for name, spec := range cases {
		sc := &ShardedCache{ObjectMeta: metav1.ObjectMeta{Name: "x"}, Spec: spec}
		if _, err := sc.ValidateCreate(); err == nil {
			t.Fatalf("%s: expected validation error", name)
		}
	}
}

func TestShardedCacheDefault(t *testing.T) {
	sc := &ShardedCache{ObjectMeta: metav1.ObjectMeta{Name: "x"}}
	sc.Default()
	if sc.Spec.Shards != 16 || sc.Spec.CacheSizePerShard != 1000 || sc.Spec.Image == "" {
		t.Fatalf("defaults not applied: %+v", sc.Spec)
	}
}
