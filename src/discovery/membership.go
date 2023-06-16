package discovery

import (
	"github.com/hashicorp/serf/serf"
	"net"
	"os"
)

const (
	ServerId      = "server_id"
	ServerAddress = "server_address"
)

func SetupSerf(serfAddress, raftAddress, rpcAddress string, serfJoinAddresses []string, eventCh chan serf.Event) (*serf.Serf, error) {
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
		ServerId:      rpcAddress,
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
