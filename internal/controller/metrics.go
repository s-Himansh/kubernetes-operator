package controller

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	reconcileTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "shardedcache_reconcile_total",
			Help: "Total ShardedCache reconciliations by result.",
		},
		[]string{"result"},
	)
)

func init() {
	metrics.Registry.MustRegister(reconcileTotal)
}

func recordReconcile(result string) {
	reconcileTotal.WithLabelValues(result).Inc()
}
