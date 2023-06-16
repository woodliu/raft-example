package service

import (
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/raft"
	raft2 "github.com/woodliu/raft-example/src/raft"
	"net"
	"os"
	"path/filepath"
	"time"

	raftboltdb "github.com/hashicorp/raft-boltdb/v2"
)

func newRaftTransport(opts *opts) (*raft.NetworkTransport, error) {
	address, err := net.ResolveTCPAddr("tcp", opts.RaftAddress)
	if err != nil {
		return nil, err
	}

	transport, err := raft.NewTCPTransport(address.String(), address, 3, 10*time.Second, os.Stderr)
	if err != nil {
		return nil, err
	}

	return transport, nil
}

func setupRaft(opts *opts, raftNotifyCh chan bool) (*raft.Raft, raft2.FSM, error) {
	raftConfig := raft.DefaultConfig()
	raftConfig.LocalID = raft.ServerID(opts.RpcAddress)
	raftConfig.Logger = hclog.New(&hclog.LoggerOptions{
		Name:       "raft",
		Output:     os.Stdout,
		TimeFormat: time.RFC3339,
		Level:      hclog.DefaultLevel,
	})
	raftConfig.SnapshotInterval = 30 * time.Second
	raftConfig.SnapshotThreshold = 100

	transport, err := newRaftTransport(opts)
	if err != nil {
		return nil, nil, err
	}

	if err := os.MkdirAll(opts.DataDir, 0700); err != nil {
		return nil, nil, err
	}

	snapshotStore, err := raft.NewFileSnapshotStore(opts.DataDir, snapshotsRetained, os.Stderr)
	if err != nil {
		return nil, nil, err
	}

	logStore, err := raftboltdb.NewBoltStore(filepath.Join(opts.DataDir, "raft-log.db"))
	if err != nil {
		return nil, nil, err
	}

	stableStore, err := raftboltdb.NewBoltStore(filepath.Join(opts.DataDir, "raft-stable.db"))
	if err != nil {
		return nil, nil, err
	}

	if opts.Bootstrap {
		hasState, err := raft.HasExistingState(logStore, stableStore, snapshotStore)
		if err != nil {
			return nil, nil, err
		}
		if !hasState {
			configuration := raft.Configuration{
				Servers: []raft.Server{
					{
						ID:      raftConfig.LocalID,
						Address: transport.LocalAddr(),
					},
				},
			}

			if err := raft.BootstrapCluster(raftConfig, logStore, stableStore, snapshotStore, transport, configuration); err != nil {
				return nil, nil, err
			}
		}
	}

	// Set up a channel for reliable leaderAddress notifications.
	raftConfig.NotifyCh = raftNotifyCh

	fsm := raft2.NewFsm()
	raftNode, err := raft.NewRaft(raftConfig, fsm, logStore, stableStore, snapshotStore, transport)
	if err != nil {
		return nil, nil, err
	}

	return raftNode, fsm, nil
}
