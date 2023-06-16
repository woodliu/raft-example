package discovery

import (
	"github.com/hashicorp/serf/serf"
	"go.uber.org/zap"
	"net"
	"os"
)

const (
	ServerAddress = "server_id"
)

func SetupSerf(serfAddress, raftAddress string, serfJoinAddresses []string, eventCh chan serf.Event) (*serf.Serf, error) {
	addr, err := net.ResolveTCPAddr("tcp", serfAddress)
	if err != nil {
		return nil, err
	}
	config := serf.DefaultConfig()
	config.Init()
	config.MemberlistConfig.BindAddr = addr.IP.String()
	config.MemberlistConfig.BindPort = addr.Port
	config.MemberlistConfig.LogOutput = os.Stdout
	config.EnableNameConflictResolution = true
	config.EventCh = eventCh
	config.Tags = map[string]string{
		"node_name":   serfAddress,
		ServerAddress: raftAddress,
	}
	config.NodeName = serfAddress
	serfNode, err := serf.Create(config)
	if err != nil {
		return nil, err
	}

	if serfJoinAddresses != nil {
		_, err = serfNode.Join(serfJoinAddresses, true)
		if err != nil {
			return nil, err
		}
	}
	return serfNode, nil
}

type Membership struct {
	serfConfig
	handler Handler
	serf    *serf.Serf
	events  chan serf.Event
	logger  *zap.Logger
}

func New(handler Handler, config serfConfig) (*Membership, error) {
	c := &Membership{
		serfConfig: config,
		handler:    handler,
		logger:     zap.L().Named("membership"),
	}
	if err := c.setupSerf(); err != nil {
		return nil, err
	}
	return c, nil
}

type serfConfig struct {
	NodeName       string
	BindAddr       string
	Tags           map[string]string
	StartJoinAddrs []string
	EventCh        chan serf.Event
}

func (m *Membership) setupSerf() (err error) {
	addr, err := net.ResolveTCPAddr("tcp", m.BindAddr)
	if err != nil {
		return err
	}
	config := serf.DefaultConfig()
	config.Init()
	config.MemberlistConfig.BindAddr = addr.IP.String()
	config.MemberlistConfig.BindPort = addr.Port
	config.MemberlistConfig.LogOutput = os.Stdout
	config.EnableNameConflictResolution = true
	m.events = make(chan serf.Event)
	config.EventCh = m.events
	config.Tags = m.Tags
	config.NodeName = m.serfConfig.NodeName
	m.serf, err = serf.Create(config)
	if err != nil {
		return err
	}
	go m.eventHandler()
	if m.StartJoinAddrs != nil {
		_, err = m.serf.Join(m.StartJoinAddrs, true)
		if err != nil {
			return err
		}
	}
	return nil
}

type Handler interface {
	Join(name, addr string) error
	Leave(name string) error
}

func (m *Membership) eventHandler() {
	for e := range m.events {
		switch e.EventType() {
		case serf.EventMemberJoin:
			for _, member := range e.(serf.MemberEvent).Members {
				if m.isLocal(member) {
					continue
				}
				m.handleJoin(member)
			}
		case serf.EventMemberLeave, serf.EventMemberFailed:
			for _, member := range e.(serf.MemberEvent).Members {
				if m.isLocal(member) {
					return
				}
				m.handleLeave(member)
			}
		}
	}
}

func (m *Membership) handleJoin(member serf.Member) {
	if err := m.handler.Join(
		member.Name,
		member.Tags["rpc_addr"],
	); err != nil {
		m.logError(err, "failed to join", member)
	}
}

func (m *Membership) handleLeave(member serf.Member) {
	if err := m.handler.Leave(
		member.Name,
	); err != nil {
		m.logError(err, "failed to leave", member)
	}
}

func (m *Membership) isLocal(member serf.Member) bool {
	return m.serf.LocalMember().Name == member.Name
}

func (m *Membership) Members() []serf.Member {
	return m.serf.Members()
}

func (m *Membership) Leave() error {
	return m.serf.Leave()
}

func (m *Membership) logError(err error, msg string, member serf.Member) {
	m.logger.Error(
		msg,
		zap.Error(err),
		zap.String("name", member.Name),
		zap.String("rpc_addr", member.Tags["rpc_addr"]),
	)
}

type handler struct {
	joins  chan map[string]string
	leaves chan string
}

func (h *handler) Join(id, addr string) error {
	if h.joins != nil {
		h.joins <- map[string]string{
			"id":   id,
			"addr": addr,
		}
	}
	return nil
}

func (h *handler) Leave(id string) error {
	if h.leaves != nil {
		h.leaves <- id
	}
	return nil
}
