package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	k8smetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	ReconcilesTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "reconciles_total",
			Help: "Number of total reconciliation attempts",
		},
	)
)

func init() {
	k8smetrics.Registry.MustRegister(ReconcilesTotal)
}
