package utils

import (
	"github.com/armon/go-metrics"
	gometric "github.com/armon/go-metrics/prometheus"
	"github.com/prometheus/client_golang/prometheus"
)

var PromSink *gometric.PrometheusSink

func NewPromRegistry() (*prometheus.Registry, error) {
	var err error
	reg := prometheus.NewRegistry()
	cfg := gometric.PrometheusOpts{
		Registerer: reg,
		Expiration: 0,
		GaugeDefinitions: []gometric.GaugeDefinition{
			{
				Name:        []string{"raft", "example"},
				Help:        "raft_example provides an example of raft",
				ConstLabels: []metrics.Label{{Name: "version", Value: "0.1"}},
			},
		},
	}
	PromSink, err = gometric.NewPrometheusSinkFrom(cfg)
	if err != nil {
		return nil, err
	}

	return reg, nil
}
