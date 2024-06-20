package raft

import (
	"context"
	"errors"
	"fmt"
	"github.com/armon/go-metrics"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/raft"
	raftboltdb "github.com/hashicorp/raft-boltdb/v2"
	"github.com/hashicorp/serf/serf"
	"github.com/woodliu/raft-example/src/discovery"
	"github.com/woodliu/raft-example/src/rpc"
	"github.com/woodliu/raft-example/src/utils"
	"go.uber.org/atomic"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"time"
)

type Raft interface {
	SetValue(date []byte) error
	GetValue(key string) string
	GetRaftStats() map[string]string

	Shutdown(ctx context.Context)
}

const (
	raftMaxPool = 3
	raftTimeout = 2 * time.Second
)

func newRaftTransport(opts *Opts) (*raft.NetworkTransport, error) {
	raftAddress, err := net.ResolveTCPAddr("tcp", opts.RaftAddress)
	if err != nil {
		return nil, err
	}

	transport, err := raft.NewTCPTransport(raftAddress.String(), raftAddress, raftMaxPool, raftTimeout, os.Stdout)
	if err != nil {
		return nil, err
	}

	return transport, nil
}

func setupRaft(loggerCtx context.Context, opts *Opts, raftNotifyCh chan bool) (*raft.Raft, FSM, error) {
	raftConfig := raft.DefaultConfig()
	raftConfig.LocalID = raft.ServerID(opts.SerfAddress)
	// Set up a channel for reliable leaderAddress notifications.
	raftConfig.NotifyCh = raftNotifyCh
	raftConfig.Logger = hclog.New(&hclog.LoggerOptions{
		Name:       "raft",
		Output:     os.Stdout,
		TimeFormat: time.RFC3339,
		Level:      hclog.DefaultLevel,
	})

	if err := os.MkdirAll(opts.DataDir, 0700); err != nil {
		return nil, nil, err
	}

	snapshotStore, err := raft.NewFileSnapshotStore(opts.DataDir, snapshotsRetained, os.Stderr)
	if err != nil {
		return nil, nil, err
	}

	// store raft log
	logStore, err := raftboltdb.NewBoltStore(filepath.Join(opts.DataDir, "raft-log.db"))
	if err != nil {
		return nil, nil, err
	}

	// store raft metadata
	stableStore, err := raftboltdb.NewBoltStore(filepath.Join(opts.DataDir, "raft-stable.db"))
	if err != nil {
		return nil, nil, err
	}

	transport, err := newRaftTransport(opts)
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

	fsm := newFsm(loggerCtx)
	raftNode, err := raft.NewRaft(raftConfig, fsm, logStore, stableStore, snapshotStore, transport)

	future := raftNode.GetConfiguration()
	configuration := future.Configuration()
	for _, v := range configuration.Servers { //For kubernetes, you need to add the endpoints serf address
		if string(v.ID) != opts.SerfAddress {
			opts.SerfJoinAddresses = append(opts.SerfJoinAddresses, string(v.ID))
		}
	}

	if err != nil {
		return nil, nil, err
	}

	return raftNode, fsm, nil
}

const (
	// serfEventBacklog is the maximum number of unprocessed Serf Events
	// that will be held in queue before new serf events block.  A
	// blocking serf event queue is a bad thing.
	serfEventBacklog = 256

	// serfEventBacklogWarning is the threshold at which point log
	// warnings will be emitted indicating a problem when processing serf
	// events.
	serfEventBacklogWarning = 200
	barrierWriteTimeout     = 2 * time.Minute

	snapshotsRetained = 2
)

type Opts struct {
	RaftAddress string
	RpcAddress  string

	DataDir   string
	Bootstrap bool

	SerfAddress       string
	SerfJoinAddresses []string
}

type raftSrv struct {
	id         string // equal to raft.Config.LocalID, setup to rpcAddress
	leaderAddr atomic.String
	isLeader   atomic.Bool

	opts       *Opts
	logger     *zap.SugaredLogger
	shutdownCh chan struct{}

	raftNotifyCh chan bool
	raftNode     *raft.Raft
	fsm          FSM

	serfNodesId sync.Map
	serfNode    *serf.Serf
	serfEventCh chan serf.Event

	applyCh        chan []byte
	forwardApplyCh chan []byte
	applyResCh     chan error
}

func NewRaft(loggerCtx context.Context, opts *Opts) Raft {
	var srv raftSrv
	srv.id = opts.SerfAddress
	srv.serfNodesId.Store(srv.id, struct{}{})
	srv.opts = opts
	srv.logger = utils.LoggerFromContext(loggerCtx).Sugar()
	srv.shutdownCh = make(chan struct{})

	srv.raftNotifyCh = make(chan bool, 10)
	raftNode, fsm, err := setupRaft(loggerCtx, srv.opts, srv.raftNotifyCh)
	if nil != err {
		srv.logger.Fatalln(err.Error())
	}
	srv.raftNode = raftNode
	srv.fsm = fsm

	srv.serfEventCh = make(chan serf.Event, serfEventBacklog)
	serfNode, err := discovery.SetupSerf(loggerCtx, srv.opts.SerfAddress, srv.id, srv.opts.RaftAddress, srv.opts.SerfJoinAddresses, srv.serfEventCh)
	if nil != err {
		srv.logger.Fatalln(err.Error())
	}
	srv.serfNode = serfNode

	srv.applyCh = make(chan []byte)
	srv.applyResCh = make(chan error)
	srv.forwardApplyCh = make(chan []byte)

	go srv.trackLeaderChanges()
	go srv.monitorLeadership()
	go srv.eventHandler()

	rpcSrv := rpc.NewRpcServer(loggerCtx, srv.opts.RpcAddress, raftNode)
	go rpcSrv.StartRpc(srv.shutdownCh)
	go srv.forwardRequest(loggerCtx)

	return &srv
}

func (srv *raftSrv) eventHandler() {
	var numQueuedEvents int
	for {
		numQueuedEvents = len(srv.serfEventCh)
		if numQueuedEvents > serfEventBacklogWarning {
			srv.logger.Warnf("The number of queued serf events [%d] is above warning threshold [%s]", numQueuedEvents, string(serfEventBacklogWarning))
		}

		select {
		case <-srv.shutdownCh:
			err := srv.serfNode.Leave()
			if err != nil {
				srv.logger.Errorf("serf node %s leave failed", srv.id)
				srv.serfNode.Shutdown()
			}
			return

		case e := <-srv.serfEventCh:
			err := srv.handleRaftMembers(e)
			if nil != err {
				srv.logger.Error(err)
			}
		}
	}
}

func (srv *raftSrv) handleRaftMembers(event serf.Event) error {
	me, ok := event.(serf.MemberEvent)
	if !ok {
		return errors.New(fmt.Sprint("Bad event type", event))
	}
	switch event.EventType() {
	case serf.EventMemberJoin, serf.EventMemberUpdate:
		if srv.isLeader.Load() {
			for _, member := range me.Members {
				utils.PromSink.IncrCounterWithLabels(utils.RaftEventMemberState, 1, []metrics.Label{{Name: "id", Value: member.Tags[discovery.ServerId]}, {Name: "state", Value: "update"}})
				if future := srv.raftNode.AddVoter(raft.ServerID(member.Tags[discovery.ServerId]), raft.ServerAddress(member.Tags[discovery.ServerAddress]), 0, 0); future.Error() != nil {
					srv.logger.Errorf("raft add node %s failed:%s", member.Tags[discovery.ServerId], future.Error())
				} else {
					srv.serfNodesId.Store(member.Tags[discovery.ServerId], struct{}{})
				}
			}
		}
	case serf.EventMemberFailed:
		for _, member := range me.Members {
			srv.serfNodesId.Delete(member.Tags[discovery.ServerId])
			if srv.isLeader.Load() {
				utils.PromSink.IncrCounterWithLabels(utils.RaftEventMemberState, 1, []metrics.Label{{Name: "id", Value: member.Tags[discovery.ServerId]}, {Name: "state", Value: "failed"}})
				srv.logger.Warnf("member failed, server id:%s", member.Tags[discovery.ServerId])
			}
		}
	case serf.EventMemberLeave:
		for _, member := range me.Members {
			if srv.isLeader.Load() {
				srv.serfNodesId.Delete(member.Tags[discovery.ServerId])
				utils.PromSink.IncrCounterWithLabels(utils.RaftEventMemberState, 1, []metrics.Label{{Name: "id", Value: member.Tags[discovery.ServerId]}, {Name: "state", Value: "leave"}})
				if future := srv.raftNode.RemoveServer(raft.ServerID(member.Tags[discovery.ServerId]), 0, 0); future.Error() != nil {
					srv.logger.Errorf("raft remove node %s failed:%s", member.Tags[discovery.ServerId], future.Error())
				}
			}
		}
	// All of these event types are ignored.
	case serf.EventUser:
	case serf.EventQuery:

	default:
		srv.logger.Warnln("Unhandled Serf Event:", event)
	}

	return nil
}

func (srv *raftSrv) reconcileMember() {
	for _, member := range srv.serfNode.Members() {
		switch member.Status {
		case serf.StatusAlive:
			if _, ok := srv.serfNodesId.Load(member.Tags[discovery.ServerId]); !ok {
				utils.PromSink.IncrCounterWithLabels(utils.RaftReconcileMemberState, 1, []metrics.Label{{Name: "id", Value: member.Tags[discovery.ServerId]}, {Name: "state", Value: "active"}})
				err := srv.raftNode.AddVoter(raft.ServerID(member.Tags[discovery.ServerId]), raft.ServerAddress(member.Tags[discovery.ServerAddress]), 0, 0).Error()
				if nil != err {
					srv.logger.Errorln("reconcile add raft failed", err)
				} else {
					srv.serfNodesId.Store(member.Tags[discovery.ServerId], struct{}{})
				}
			}

		case serf.StatusFailed:
			srv.serfNodesId.Delete(member.Tags[discovery.ServerId])
			utils.PromSink.IncrCounterWithLabels(utils.RaftReconcileMemberState, 1, []metrics.Label{{Name: "id", Value: member.Tags[discovery.ServerId]}, {Name: "state", Value: "failed"}})
			srv.logger.Errorf("serf status failed, server address:%s", member.Tags[discovery.ServerAddress])
		case serf.StatusLeft:
			srv.serfNodesId.Delete(member.Tags[discovery.ServerId])
			utils.PromSink.IncrCounterWithLabels(utils.RaftReconcileMemberState, 1, []metrics.Label{{Name: "id", Value: member.Tags[discovery.ServerId]}, {Name: "state", Value: "left"}})
			err := srv.raftNode.RemoveServer(raft.ServerID(member.Tags[discovery.ServerId]), 0, 0).Error()
			if nil != err {
				srv.logger.Errorln("reconcile remove raft failed", err)
			}
		}
	}
}

// save leader id to srv.leaderAddr
func (srv *raftSrv) trackLeaderChanges() {
	obsLeader := make(chan raft.Observation, 16)
	observer := raft.NewObserver(obsLeader, false, func(o *raft.Observation) bool {
		_, ok := o.Data.(raft.LeaderObservation)
		return ok
	})
	srv.raftNode.RegisterObserver(observer)

	for {
		select {
		case obs := <-obsLeader:
			leaderObs, ok := obs.Data.(raft.LeaderObservation)
			if !ok {
				srv.logger.Debugf(fmt.Sprint("got unknown observation type from raft", "type", reflect.TypeOf(obs.Data)))
				continue
			}

			utils.PromSink.IncrCounterWithLabels(utils.RaftLeaderChangeTotal, 1, []metrics.Label{{Name: "leader", Value: string(leaderObs.LeaderID)}})
			srv.leaderAddr.Store(string(leaderObs.LeaderID))
		case <-srv.shutdownCh:
			srv.raftNode.DeregisterObserver(observer)
			return
		}
	}
}

// check if this node is leader, and save to srv.isLeader
func (srv *raftSrv) monitorLeadership() {
	ctx, cancel := context.WithCancel(context.Background())
	var leaderLoop errgroup.Group
	for {
		select {
		case isLeader := <-srv.raftNotifyCh:
			switch {
			case isLeader:
				if srv.isLeader.CAS(false, true) {
					utils.PromSink.SetGauge(utils.RaftIsLeader, 1)
					ctx, cancel = context.WithCancel(context.Background())
					leaderLoop.Go(func() error {
						return srv.leaderLoop(ctx)
					})
					srv.logger.Infoln("cluster leadership acquired")
				}

			default:
				if srv.isLeader.CAS(true, false) {
					utils.PromSink.SetGauge(utils.RaftIsLeader, 0)
					srv.logger.Debugln("shutting down leaderAddress loop")
					cancel()
					leaderLoop.Wait()
					srv.logger.Infoln("cluster leadership lost")
				}
			}
		case <-srv.shutdownCh:
			return
		}
	}
}

func (srv *raftSrv) leaderLoop(ctx context.Context) error {
RECONCILE:
	// Setup a reconciliation timer
	interval := time.NewTicker(60 * time.Second)

	// Apply a raft barrier to ensure our FSM is caught up
	barrier := srv.raftNode.Barrier(barrierWriteTimeout)
	if err := barrier.Error(); err != nil {
		srv.logger.Errorln("failed to wait for barrier", "error", err)
		goto WAIT
	}

WAIT:
	if !srv.isLeader.Load() {
		return nil
	}

	// Periodically reconcile as long as we are the leaderAddress,
	// or when Serf events arrive
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-srv.shutdownCh:
			return nil
		case <-interval.C:
			srv.reconcileMember()
			goto RECONCILE
		case data := <-srv.applyCh:
			srv.applyResCh <- srv.raftNode.Apply(data, utils.EnqueueLimit).Error()
		}
	}
}

func (srv *raftSrv) forwardRequest(loggerCtx context.Context) {
	for {
		select {
		case data := <-srv.forwardApplyCh:
			conn, err := rpc.CreateLeaderConn(loggerCtx, srv.leaderAddr.Load())
			if nil != err {
				srv.applyResCh <- err
			} else {
				srv.applyResCh <- rpc.Apply(conn, data)
			}
			conn.Close()
		case <-srv.shutdownCh:
			return
		}
	}
}

func (srv *raftSrv) SetValue(date []byte) error {
	if srv.isLeader.Load() { // if current node is leader, do raft.Apply directly
		s := time.Now()
		srv.applyCh <- date
		utils.PromSink.AddSampleWithLabels(utils.RaftApplyLatencySummary, float32(time.Since(s).Seconds()), []metrics.Label{{Name: "type", Value: "leader"}})
	} else {
		s := time.Now()
		srv.forwardApplyCh <- date
		utils.PromSink.AddSampleWithLabels(utils.RaftApplyLatencySummary, float32(time.Since(s).Seconds()), []metrics.Label{{Name: "type", Value: "forward"}})
	}

	if err := <-srv.applyResCh; nil != err {
		return err
	}

	return nil
}

func (srv *raftSrv) GetValue(key string) string {
	return srv.fsm.Get(key)
}

func (srv *raftSrv) GetRaftStats() map[string]string {
	return srv.raftNode.Stats()
}

func (srv *raftSrv) Shutdown(loggerCtx context.Context) {
	logger := utils.LoggerFromContext(loggerCtx).Sugar()
	if !srv.isLeader.Load() {
		conn, err := rpc.CreateLeaderConn(loggerCtx, srv.leaderAddr.Load())
		if err != nil {
			logger.Errorf("create connection to raft server %s failed %v", srv.leaderAddr.Load(), err)
			goto Exit
		}
		err = rpc.RemoveServer(conn, srv.id)
		if err != nil {
			logger.Errorf("forward remove raft server %s failed %v", srv.leaderAddr.Load(), err.Error())
		}
		//} else {
		//	future := srv.raftNode.RemoveServer(raft.ServerID(srv.id), 0, 0) //doesn't remove leader itself, serf will remove it when timeout
		//	if err := future.Error(); err != nil {
		//		logger.Errorf("remove leader %s failed %v", srv.leaderAddr.Load(), err.Error())
		//	}
	}

Exit:
	srv.raftNode.Shutdown()
	close(srv.shutdownCh)
}
