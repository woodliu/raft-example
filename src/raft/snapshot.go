package raft

import (
	"github.com/armon/go-metrics"
	"github.com/hashicorp/raft"
	"github.com/woodliu/raft-example/src/utils"
)

type snapshot struct {
	fsm FSM
}

// Persist saves the FSM snapshot out to the given sink.
func (s *snapshot) Persist(sink raft.SnapshotSink) error {
	snapshotBytes, err := s.fsm.Marshal()
	if err != nil {
		utils.PromSink.IncrCounterWithLabels(utils.FsmPersistTotal, 1, []metrics.Label{{Name: "state", Value: "failed"}})
		sink.Cancel()
		return err
	}

	if _, err := sink.Write(snapshotBytes); err != nil {
		utils.PromSink.IncrCounterWithLabels(utils.FsmPersistTotal, 1, []metrics.Label{{Name: "state", Value: "failed"}})
		sink.Cancel()
		return err
	}

	if err := sink.Close(); err != nil {
		utils.PromSink.IncrCounterWithLabels(utils.FsmPersistTotal, 1, []metrics.Label{{Name: "state", Value: "failed"}})
		sink.Cancel()
		return err
	}
	utils.PromSink.IncrCounterWithLabels(utils.FsmPersistTotal, 1, []metrics.Label{{Name: "state", Value: "success"}})
	return nil
}

func (s *snapshot) Release() {
}
