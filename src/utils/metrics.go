package utils

import (
	"github.com/armon/go-metrics"
	gometric "github.com/armon/go-metrics/prometheus"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	RaftEventMemberState     = []string{"raft", "member", "event", "state"}
	RaftReconcileMemberState = []string{"raft", "member", "reconcile", "state"}

	RaftLeaderChangeTotal   = []string{"raft", "leader", "change", "total"}
	RaftIsLeader            = []string{"raft", "is", "leader"}
	RaftApplyLatencySummary = []string{"raft", "apply", "seconds", "summary"}
	RaftLatencySummary      = []string{"raft", "latency", "seconds", "summary"}

	FsmApplyLatencySummary = []string{"fsm", "apply", "seconds", "summary"}
	FsmPersistTotal        = []string{"fsm", "persist", "total"}
	FsmRestoreTotal        = []string{"fsm", "restore", "total"}

	RpcForwardLatencySummary = []string{"rpc", "forward", "seconds", "summary"}
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
