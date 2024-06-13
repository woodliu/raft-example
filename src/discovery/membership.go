package discovery

import (
	"context"
	"errors"
	"github.com/hashicorp/serf/serf"
	"github.com/woodliu/raft-example/cli"
	"github.com/woodliu/raft-example/src/utils"
	"go.uber.org/zap"
	"net"
	"os"
	"time"
)

const (
	ServerId      = "server_id"
	ServerAddress = "server_address"
)

type serfNode struct {
	logger *zap.SugaredLogger
	serf   *serf.Serf
}

func newSerf(loggerCtx context.Context, serfAddress, serverId, serverAddress string, eventCh chan serf.Event) (*serfNode, error) {
	logger := utils.LoggerFromContext(loggerCtx).Sugar()
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
		ServerId:      serverId,
		ServerAddress: serverAddress,
	}
	config.NodeName = serfAddress
	node, err := serf.Create(config)
	if err != nil {
		return nil, err
	}

	return &serfNode{logger: logger, serf: node}, nil
}

func (s *serfNode) join(joinAddresses []string) (int, error) {
	if joinAddresses != nil {
		return s.serf.Join(joinAddresses, true)
	}
	return 0, nil
}

func SetupSerf(loggerCtx context.Context, serfAddress, serverId, serverAddress string, joinAddresses []string, eventCh chan serf.Event) (*serf.Serf, error) {
	node, err := newSerf(loggerCtx, serfAddress, serverId, serverAddress, eventCh)
	if err != nil {
		return nil, err
	}

	if joinAddresses != nil {
		// Track the number of join attempts
		attempt := 0
		for {
			// Try to perform the join
			node.logger.Infof("Joining cluster...")
			_, err := node.serf.Join(joinAddresses, true)
			if err == nil {
				return node.serf, nil
			}

			// Check if the maximum attempts has been exceeded
			attempt++
			if cli.RetryMaxAttempts > 0 && attempt > cli.RetryMaxAttempts {
				return nil, errors.New("maximum retry join attempts made, exiting")
			}
			// Log the failure and sleep
			node.logger.Warnf("Join failed: %v, retrying in %v", err, cli.RetryInterval)
			time.Sleep(time.Second * time.Duration(cli.RetryInterval))
		}
	}

	return node.serf, nil
}
