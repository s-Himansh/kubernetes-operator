# kubernetes-operator

Kubernetes Operator for `ShardedCache` — extends Kubernetes to manage the [shared-lru-cache](https://github.com/s-Himansh/shared-lru-cache) deployments declaratively. Capstone project (#6) from the learning roadmap: **Tier 3 — K8s Internals → Extender**.

> **Goal:** Prove platform-engineering depth: CRD + controller-runtime reconciliation, status conditions, RBAC, validation webhooks, Helm + ArgoCD GitOps, and envtest e2e.

## Architecture

```
ShardedCache CR (desired: shards=16, cacheSize=1000)
        │
        ▼
┌─────────────────┐   reconciles   ┌──────────────┐  ┌─────────┐
│  Operator       │ ──────────────▶│ Deployment   │─▶│  Pods   │
│  (controller)   │                │ + Service    │  │ (cache) │
└─────────────────┘   status       └──────────────┘  └─────────┘
        ▲              conditions        │
        └───────────────────────────────┘
```

* **CRD:** `cache.example.com/v1, Kind=ShardedCache` — desired shard count, per-shard capacity, image.
* **Reconciliation:** desired vs actual shard count → create/update Deployment (replicas = shards) + headless Service, scale up/down, emit Events.
* **Status:** `Ready`, `Progressing`, `Degraded` conditions + `readyShards`, `observedGeneration`.

## Quick Start

```bash
# prerequisites: go 1.22+, docker, kind/k3d or any cluster, kubectl, helm

# 1. install CRD
make install

# 2. run operator locally (outside cluster)
make run

# 3. apply sample
kubectl apply -f config/samples/cache_v1_shardedcache.yaml
kubectl get shardedcaches -o yaml

# 4. deploy via Helm (includes CRD + RBAC + manager)
helm upgrade --install sharded-cache ./charts/kubernetes-operator -n cache-system --create-namespace

# 5. ArgoCD GitOps
kubectl apply -f config/argocd/application.yaml
```

## CRD Spec

```yaml
apiVersion: cache.example.com/v1
kind: ShardedCache
metadata:
  name: demo
spec:
  shards: 8                    # desired shards (1-64)
  cacheSizePerShard: 512       # entries per shard
  image: ghcr.io/s-himansh/shared-lru-cache:latest
  version: v0.1.0
  resources:                   # optional
    requests: {cpu: 100m, memory: 128Mi}
    limits: {cpu: 500m, memory: 512Mi}
status:
  readyShards: 8
  phase: Ready
  conditions:
    - type: Ready
      status: "True"
```

## Project Layout

```
api/v1/                 # CRD types (ShardedCache)
internal/controller/    # reconciliation loop
config/                 # kustomize bases: crd, rbac, manager, samples
charts/                 # Helm chart (alternative to kustomize)
cmd/main.go             # manager entrypoint
```

## Development

```bash
make manifests   # generate CRD + RBAC via controller-gen
make generate    # generate DeepCopy
make test        # unit + envtest (needs etcd/kube-apiserver)
make docker-build IMG=ghcr.io/s-himansh/kubernetes-operator:latest
make deploy      # kustomize deploy to current kube-context
make helm-template  # helm template render check
```

## Reconciliation Semantics

* **Idempotent:** requeue on Deployment drift; patch don't replace.
* **Scale:** shards ↑ → `deployment.spec.replicas` ↑ ; shards ↓ → scale down.
* **Ownership:** Deployment/Service owned by ShardedCache (ownerReference) → GC on CR deletion.
* **Finalizer:** `cache.example.com/finalizer` ensures graceful cleanup (optional, enabled via flag).
* **Events:** `Normal` on create/update/scale, `Warning` on invalid spec (validated by webhook).

## Webhooks

* **Defaulting:** `shards` defaults to 16, `cacheSizePerShard` to 1000, `image` to `ghcr.io/s-himansh/shared-lru-cache:latest`.
* **Validation:** `1 <= shards <= 64`, `cacheSizePerShard > 0`, image must be non-empty.

## Observability

* Prometheus metrics at `:8080/metrics` (controller-runtime + custom `shardedcache_reconcile_total`).
* ServiceMonitor at `config/manager/servicemonitor.yaml` for Prometheus Operator (30s scrape).
* Grafana dashboard: `config/grafana/dashboard.json` (reconcile rate, queue depth, ready shards).
* Liveness/Readiness probes on manager.

## Testing

* `internal/controller/suite_test.go` — envtest with real etcd + apiserver (no mock).
* `api/v1/*_test.go` — webhook validation unit tests.

## Roadmap vs Resume

Ties portfolio together: operator manages *your own* Sharded LRU Cache — demonstrates K8s API machinery, operator pattern, and GitOps, closing the "Kubernetes internals" skill gap.
