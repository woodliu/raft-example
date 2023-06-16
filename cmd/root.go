package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "raft-example",
	Short: "A raft demo",
	Long:  `A raft demo`,
}

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

var (
	HttpAddress string
	RaftAddress string
	RpcAddress  string

	DataDir   string
	Bootstrap bool

	SerfAddress       string
	SerfJoinAddresses []string
)

func init() {
	rootCmd.PersistentFlags().StringVar(&HttpAddress, "httpAddress", "", "Http server listen address, handling user request")

	rootCmd.PersistentFlags().StringVar(&RaftAddress, "raftTCPAddress", "", "Raft server listen address")
	rootCmd.PersistentFlags().StringVar(&RpcAddress, "rpcAddress", "", "For forwarding http request through grpc to raft leader")
	rootCmd.PersistentFlags().StringVar(&DataDir, "dataDir", "", "Raft data folder path")
	rootCmd.PersistentFlags().BoolVar(&Bootstrap, "bootstrap", false, "whether this raft node need to bootstrap")

	rootCmd.PersistentFlags().StringVar(&SerfAddress, "serfAddress", "", "Serf server listen address")
	rootCmd.PersistentFlags().StringArrayVar(&SerfJoinAddresses, "joinAddress", []string{}, "address to join serf cluster")
}
