package cli

import (
	"errors"
	"github.com/urfave/cli/v2"
	"log"
	"net"
	"os"
	"sort"
)

var (
	HttpAddress string
	RaftAddress string
	RpcAddress  string

	DataDir   string
	Bootstrap bool

	SerfAddress       string
	SerfJoinAddresses cli.StringSlice
	RetryMaxAttempts  int
	RetryInterval     int64
)

func validateCliAdder(addr string) error {
	tcpAddr, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		return err
	}
	if tcpAddr.IP == nil {
		return errors.New("invalid address")
	}
	return nil
}
func init() {
	app := &cli.App{
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "http-address",
				Required:    true,
				Usage:       "Http server listen address, handling user request. Format: [IP:PORT]",
				Destination: &HttpAddress,
				Category:    "Http server",
				Action: func(ctx *cli.Context, v string) error {
					return validateCliAdder(v)
				},
			},
			&cli.StringFlag{
				Name:        "raft-address",
				Required:    true,
				Usage:       "Raft server listen address",
				Destination: &RaftAddress,
				Category:    "Raft",
				Action: func(ctx *cli.Context, v string) error {
					return validateCliAdder(v)
				},
			},
			&cli.StringFlag{
				Name:        "rpc-address",
				Required:    true,
				Usage:       "For forwarding http request through grpc to raft leader. Format: [IP:PORT]",
				Destination: &RpcAddress,
				Category:    "Raft",
				Action: func(ctx *cli.Context, v string) error {
					return validateCliAdder(v)
				},
			},
			&cli.StringFlag{
				Name:        "data-dir",
				Required:    true,
				Usage:       "Raft data directory",
				Destination: &DataDir,
				Category:    "Raft",
			},
			&cli.BoolFlag{
				Name:        "bootstrap",
				Required:    false,
				Value:       false,
				Usage:       "Whether this raft node need to bootstrap",
				Destination: &Bootstrap,
				Category:    "Raft",
			},
			&cli.StringFlag{
				Name:        "serf-address",
				Required:    true,
				Usage:       "Serf server listen address. Format: [IP:PORT]",
				Destination: &SerfAddress,
				Category:    "Serf",
				Action: func(ctx *cli.Context, v string) error {
					return validateCliAdder(v)
				},
			},
			&cli.StringSliceFlag{
				Name:        "join-address",
				Required:    false,
				Usage:       "Address to join serf cluster. Format: [IP:PORT]",
				Destination: &SerfJoinAddresses,
				Category:    "Serf",
				Action: func(ctx *cli.Context, v []string) error {
					if v == nil || len(v) == 0 {
						return errors.New("join address is nil")
					}
					for _, vv := range v {
						if err := validateCliAdder(vv); err != nil {
							return err
						}
					}
					return nil
				},
			},
			&cli.IntFlag{
				Name:        "max-retry-join",
				Required:    false,
				Value:       0,
				Usage:       "limit the maximum attempts made by RetryJoin to reach other nodes. If this is 0, then no limit is imposed, and Serf will continue to try forever. Default 0",
				Destination: &RetryMaxAttempts,
				Category:    "Serf",
				Action: func(ctx *cli.Context, v int) error {
					if !(v >= 0) {
						return errors.New("invalid value")
					}
					return nil
				},
			},
			&cli.Int64Flag{
				Name:        "RetryInterval",
				Required:    false,
				Value:       30,
				Usage:       "This interval controls how often we retry the join for RetryJoin. This defaults to 30 seconds.",
				Destination: &RetryInterval,
				Category:    "Serf",
				Action: func(ctx *cli.Context, v int64) error {
					if v < 0 {
						return errors.New("invalid value")
					}
					return nil
				},
			},
		},
	}

	sort.Sort(cli.FlagsByName(app.Flags))
	sort.Sort(cli.CommandsByName(app.Commands))

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
