package service

import (
	"errors"
	"fmt"
	"github.com/gofiber/fiber/v2"
	"github.com/hashicorp/raft"
	"github.com/hashicorp/serf/serf"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/pkgerrors"
	"github.com/woodliu/raft-example/cmd"
	"github.com/woodliu/raft-example/src/discovery"
	raft2 "github.com/woodliu/raft-example/src/raft"
	"github.com/woodliu/raft-example/src/rpc"
	"go.uber.org/atomic"
	"os"
	"reflect"
	"sync"
	"time"
)

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

var Logger zerolog.Logger

type opts struct {
	RaftAddress string
	RpcAddress  string

	DataDir   string
	Bootstrap bool

	SerfAddress       string
	SerfJoinAddresses []string
}

func setOpts() *opts {
	return &opts{
		RaftAddress: cmd.RaftAddress,
		RpcAddress:  cmd.RpcAddress,

		DataDir:   cmd.DataDir,
		Bootstrap: cmd.Bootstrap,

		SerfAddress:       cmd.SerfAddress,
		SerfJoinAddresses: cmd.SerfJoinAddresses,
	}
}

type Service interface {
	SetValue(c *fiber.Ctx) error
	GetValue(c *fiber.Ctx) error
	GetRaftStats(c *fiber.Ctx) error
	Shutdown()
}

type service struct {
	opts       *opts
	logger     zerolog.Logger
	shutdownCh chan struct{}

	raftNotifyCh  chan bool
	raftNode      *raft.Raft
	fsm           raft2.FSM
	isLeader      atomic.Bool
	leaderAddress atomic.String

	serfNode    *serf.Serf
	serfEventCh chan serf.Event

	applyCh        chan []byte
	forwardApplyCh chan []byte
	applyResCh     chan error
}

func SetUpServer() Service {
	var srv service
	srv.opts = setOpts()
	zerolog.ErrorStackMarshaler = pkgerrors.MarshalStack
	srv.logger = zerolog.New(os.Stdout).With().Timestamp().Caller().Logger()
	srv.shutdownCh = make(chan struct{})

	srv.raftNotifyCh = make(chan bool, 10)
	raftNode, fsm, err := setupRaft(srv.opts, srv.raftNotifyCh)
	if nil != err {
		srv.logger.Fatal().Err(err)
	}
	srv.raftNode = raftNode
	srv.fsm = fsm

	srv.serfEventCh = make(chan serf.Event, serfEventBacklog)
	serfNode, err := discovery.SetupSerf(srv.opts.SerfAddress, srv.opts.RaftAddress, srv.opts.SerfJoinAddresses, srv.serfEventCh)
	if nil != err {
		srv.logger.Fatal().Err(err)
	}
	srv.serfNode = serfNode

	srv.applyCh = make(chan []byte)
	srv.forwardApplyCh = make(chan []byte)
	srv.applyResCh = make(chan error)

	go srv.eventHandler()
	go rpc.NewRpcServer(srv.opts.RpcAddress, raftNode).StartRpc()
	go srv.trackLeaderChanges()

	go srv.monitorLeadership()
	go srv.forwardRequest() //forwardRequest should wait RpcServer

	return &srv
}

func (srv *service) eventHandler() {
	var numQueuedEvents int
	for {
		numQueuedEvents = len(srv.serfEventCh)
		if numQueuedEvents > serfEventBacklogWarning {
			srv.logger.Warn().Str("queued_events", string(numQueuedEvents)).Str("warning_threshold", string(serfEventBacklogWarning)).Msg("number of queued serf events above warning threshold")
		}

		select {
		case <-srv.shutdownCh:
			return

		case e := <-srv.serfEventCh:
			err := srv.handleRaftMembers(e)
			if nil != err {
				srv.logger.With().Stack().Err(err)
			}
		}
	}
}

func (srv *service) handleRaftMembers(event serf.Event) error {
	me, ok := event.(serf.MemberEvent)
	if !ok {
		return errors.New(fmt.Sprint("Bad event type", event))
	}

	switch event.EventType() {
	case serf.EventMemberJoin, serf.EventMemberUpdate:
		for _, member := range me.Members {
			if future := srv.raftNode.AddVoter(raft.ServerID(member.Tags[discovery.ServerAddress]), raft.ServerAddress(member.Tags[discovery.ServerAddress]), 0, 0); future.Error() != nil {
				srv.logger.With().Err(future.Error()).Stack()
			}
		}
	case serf.EventMemberFailed:
		for _, member := range me.Members {
			srv.logger.Warn().Msgf("member failed, service:%s", member.Tags[discovery.ServerAddress])
		}
	case serf.EventMemberLeave:
		for _, member := range me.Members {
			if future := srv.raftNode.RemoveServer(raft.ServerID(member.Tags[discovery.ServerAddress]), 0, 0); future.Error() != nil {
				srv.logger.Err(future.Error()).Stack()
			}
		}
	// All of these event types are ignored.
	case serf.EventUser:
	case serf.EventQuery:

	default:
		srv.logger.Warn().Msg(fmt.Sprint("Unhandled Serf Event", "event", event))
	}

	return nil
}

func (srv *service) reconcileMember() {
	for _, member := range srv.serfNode.Members() {
		switch member.Status {
		case serf.StatusAlive:
			err := srv.raftNode.AddVoter(raft.ServerID(member.Tags[discovery.ServerAddress]), raft.ServerAddress(member.Tags[discovery.ServerAddress]), 0, 0).Error()
			if nil != err {
				srv.logger.Error().Stack().Err(err)
			}
		case serf.StatusFailed:
			srv.logger.Error().Msgf("serf status failed, service:%s", member.Tags[discovery.ServerAddress])
		case serf.StatusLeft:
			err := srv.raftNode.RemoveServer(raft.ServerID(member.Tags[discovery.ServerAddress]), 0, 0).Error()
			if nil != err {
				srv.logger.Error().Stack().Err(err)
			}
		}
	}
}

func (srv *service) trackLeaderChanges() {
	obsCh := make(chan raft.Observation, 16)
	observer := raft.NewObserver(obsCh, false, func(o *raft.Observation) bool {
		_, ok := o.Data.(raft.LeaderObservation)
		return ok
	})
	srv.raftNode.RegisterObserver(observer)

	for {
		select {
		case obs := <-obsCh:
			leaderObs, ok := obs.Data.(raft.LeaderObservation)
			if !ok {
				srv.logger.Debug().Msg(fmt.Sprint("got unknown observation type from raft", "type", reflect.TypeOf(obs.Data)))
				continue
			}

			trackLeaderAddress := string(leaderObs.LeaderAddr)
			if srv.leaderAddress.Load() != trackLeaderAddress {
				srv.leaderAddress.Store(trackLeaderAddress)
			}
		case <-srv.shutdownCh:
			srv.raftNode.DeregisterObserver(observer)
			return
		}
	}
}

func (srv *service) monitorLeadership() {
	var weAreLeaderCh chan struct{}
	var leaderLoop sync.WaitGroup
	for {
		select {
		case isLeader := <-srv.raftNotifyCh:
			switch {
			case isLeader:
				srv.isLeader.Store(true) // current node is leader
				if weAreLeaderCh != nil {
					srv.logger.Error().Msg("attempted to start the leaderAddress loop while running")
					continue
				}

				weAreLeaderCh = make(chan struct{})
				leaderLoop.Add(1)
				go func(ch chan struct{}) {
					defer leaderLoop.Done()
					srv.leaderLoop(ch) // leaderloop
				}(weAreLeaderCh)
				srv.logger.Info().Msg("cluster leadership acquired")

			default:
				srv.isLeader.Store(false) // current node is not leader
				if weAreLeaderCh == nil {
					srv.logger.Error().Msg("attempted to stop the leaderAddress loop while not running")
					continue
				}

				srv.logger.Debug().Msg("shutting down leaderAddress loop")
				close(weAreLeaderCh)
				leaderLoop.Wait()
				weAreLeaderCh = nil
				srv.logger.Info().Msg("cluster leadership lost")
			}
		case <-srv.shutdownCh:
			return
		}
	}
}

func (srv *service) leaderLoop(stopCh chan struct{}) {
RECONCILE:
	// Setup a reconciliation timer
	interval := time.NewTicker(60 * time.Second)

	// Apply a raft barrier to ensure our FSM is caught up
	barrier := srv.raftNode.Barrier(barrierWriteTimeout)
	if err := barrier.Error(); err != nil {
		srv.logger.Error().Msgf("failed to wait for barrier", "error", err)
		goto WAIT
	}

WAIT:
	// Poll the stop channel to give it priority so we don't waste time
	// trying to perform the other operations if we have been asked to shut
	// down.
	select {
	case <-stopCh:
		return
	default:
	}

	srv.reconcileMember()
	// Periodically reconcile as long as we are the leaderAddress,
	// or when Serf events arrive
	for {
		select {
		case <-stopCh:
			return
		case <-srv.shutdownCh:
			return
		case <-interval.C:
			goto RECONCILE
		case data := <-srv.applyCh:
			srv.applyResCh <- srv.raftNode.Apply(data, raft2.EnqueueLimit).Error()
		}
	}
}

func (srv *service) forwardRequest() {
	for {
		select {
		case data := <-srv.forwardApplyCh:
			conn, err := rpc.CreateLeaderConn(srv.leaderAddress.Load())
			if nil != err {
				srv.applyResCh <- err
			} else {
				srv.applyResCh <- rpc.ForwardRequest(conn, data)
			}
			conn.Close()
		case <-srv.shutdownCh:
			return
		}
	}
}

// SetValue
//
//	{
//	  "key" : "key",
//	  "value" : "value"
//	}
func (srv *service) SetValue(c *fiber.Ctx) error {
	if srv.isLeader.CAS(true, true) { // if current node is leader, do raft.Apply directly
		srv.applyCh <- c.Body()
	} else {
		srv.forwardApplyCh <- c.Body()
	}

	if err := <-srv.applyResCh; nil != err {
		srv.logger.Error().Stack().Err(err)
		c.WriteString(err.Error())
		c.SendStatus(fiber.StatusInternalServerError)
		return err
	}

	return c.SendStatus(fiber.StatusOK)
}

// GetValue
//
//	{
//	  "key" : "key"
//	}
func (srv *service) GetValue(c *fiber.Ctx) error {
	req := raft2.LogEntryData{}
	if err := c.BodyParser(&req); err != nil {
		c.Status(fiber.StatusBadRequest)
		return err
	}

	c.WriteString(srv.fsm.Get(req.Key))
	return c.SendStatus(fiber.StatusOK)
}

func (srv *service) GetRaftStats(c *fiber.Ctx) error {
	stats := srv.raftNode.Stats()
	return c.Status(fiber.StatusOK).JSON(stats)
}

func (srv *service) Shutdown() {
	srv.shutdownCh <- struct{}{}
}
