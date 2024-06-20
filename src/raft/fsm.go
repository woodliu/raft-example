package raft

import (
	"context"
	"github.com/armon/go-metrics"
	"github.com/goccy/go-json"
	"github.com/hashicorp/raft"
	"github.com/woodliu/raft-example/src/utils"
	"go.uber.org/zap"
	"io"
	"sync"
	"time"
)

type FSM interface {
	Get(key string) string
	Marshal() ([]byte, error)
}

type Fsm struct {
	data   *FsmData
	logger *zap.SugaredLogger
}

// Custom Implementation begin
type FsmData struct {
	entries map[string]string
	sync.RWMutex
}

func (f *Fsm) Set(k, v string) error {
	f.data.Lock()
	defer f.data.Unlock()
	f.data.entries[k] = v
	return nil
}

func (f *Fsm) Get(k string) string {
	f.data.RLock()
	defer f.data.RUnlock()
	return f.data.entries[k]
}

// Marshal serializes cache data
func (f *Fsm) Marshal() ([]byte, error) {
	f.data.RLock()
	defer f.data.RUnlock()
	dataBytes, err := json.Marshal(f.data.entries)
	return dataBytes, err
}

// UnMarshal deserializes cache data
func (f *Fsm) UnMarshal(serialized io.ReadCloser) error {
	defer serialized.Close()

	var newData map[string]string
	if err := json.NewDecoder(serialized).Decode(&newData); err != nil {
		return err
	}

	f.data.Lock()
	f.data.entries = newData
	f.data.Unlock()
	return nil
}

func newFsm(ctx context.Context) *Fsm {
	return &Fsm{
		data: &FsmData{
			entries: make(map[string]string),
		},
		logger: utils.LoggerFromContext(ctx).Sugar(),
	}
}

type LogEntryData struct {
	Key   string `json:"key"`
	Value string `json:"value,omitempty"`
}

// Apply applies a Raft log entry to the key-value store.
func (f *Fsm) Apply(logEntry *raft.Log) interface{} {
	log := LogEntryData{}
	// Because the data come from http body and encode as json, so decode as json here
	if err := json.Unmarshal(logEntry.Data, &log); err != nil {
		f.logger.Errorln("Unmarshalling fsm entries failed")
		return err
	}

	s := time.Now()
	ret := f.Set(log.Key, log.Value)
	utils.PromSink.AddSample(utils.FsmApplyLatencySummary, float32(time.Since(s).Seconds()))
	f.logger.Infof("fms.Apply(), logEntry:%s, ret:%v\n", logEntry.Data, ret)
	return ret
}

// Custom Implementation end

// Snapshot returns the latest snapshot
func (f *Fsm) Snapshot() (raft.FSMSnapshot, error) {
	return &snapshot{fsm: f}, nil
}

// Restore stores the key-value store to a previous state.
func (f *Fsm) Restore(serialized io.ReadCloser) error {
	err := f.UnMarshal(serialized)
	if err != nil {
		utils.PromSink.IncrCounterWithLabels(utils.FsmRestoreTotal, 1, []metrics.Label{{Name: "state", Value: "failed"}})
		return err
	}
	utils.PromSink.IncrCounterWithLabels(utils.FsmRestoreTotal, 1, []metrics.Label{{Name: "state", Value: "success"}})
	return nil
}
