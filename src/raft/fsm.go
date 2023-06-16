package raft

import (
	"encoding/json"
	"github.com/hashicorp/raft"
	"github.com/rs/zerolog"
	"io"
	"os"
	"sync"
)

type FSM interface {
	Get(key string) string
}

type Fsm struct {
	data   *FsmData
	logger zerolog.Logger
}

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
	f.data.Lock()
	defer f.data.Unlock()
	return f.data.entries[k]
}

// Marshal serializes cache data
func (fData *FsmData) Marshal() ([]byte, error) {
	fData.RLock()
	defer fData.RUnlock()
	dataBytes, err := json.Marshal(fData.entries)
	return dataBytes, err
}

// UnMarshal deserializes cache data
func (fData *FsmData) UnMarshal(serialized io.ReadCloser) error {
	defer serialized.Close()

	var newData map[string]string
	if err := json.NewDecoder(serialized).Decode(&newData); err != nil {
		return err
	}

	fData.Lock()
	fData.entries = newData
	fData.Unlock()
	return nil
}

func NewFsm() *Fsm {
	return &Fsm{
		data: &FsmData{
			entries: make(map[string]string),
		},
		logger: zerolog.New(os.Stdout).With().Timestamp().Caller().Logger().With().Str("component", "fsm").Logger(),
	}
}

type LogEntryData struct {
	Key   string `json:"key"`
	Value string `json:"value,omitempty"`
}

// Apply applies a Raft log entry to the key-value store.
func (f *Fsm) Apply(logEntry *raft.Log) interface{} {
	log := LogEntryData{}
	if err := json.Unmarshal(logEntry.Data, &log); err != nil {
		f.logger.Error().Stack().Msg("Unmarshalling fsm entries failed")
		return err
	}

	ret := f.Set(log.Key, log.Value)
	f.logger.Info().Msgf("fms.Apply(), logEntry:%s, ret:%v\n", logEntry.Data, ret)
	return ret
}

// Snapshot returns a latest snapshot
func (f *Fsm) Snapshot() (raft.FSMSnapshot, error) {
	return &snapshot{data: f.data}, nil
}

// Restore stores the key-value store to a previous state.
func (f *Fsm) Restore(serialized io.ReadCloser) error {
	return f.data.UnMarshal(serialized)
}
