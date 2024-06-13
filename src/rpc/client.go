package rpc

import (
	"context"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/retry"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/timeout"
	"github.com/woodliu/raft-example/src/utils"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"time"
)

var (
	rpcForwardLatency = []string{"rpc", "forward", "seconds"}
)

func CreateLeaderConn(loggerCtx context.Context, leaderAddr string) (*grpc.ClientConn, error) {
	opts := []retry.CallOption{
		retry.WithBackoff(retry.BackoffExponential(100 * time.Millisecond)),
	}
	conn, err := grpc.NewClient(
		leaderAddr,
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithChainUnaryInterceptor(
			retry.UnaryClientInterceptor(opts...),
			timeout.UnaryClientInterceptor(3*time.Second),
			logging.UnaryClientInterceptor(interceptorLogger(utils.LoggerFromContext(loggerCtx)), loggingOpts...)),
	)
	if err != nil {
		return nil, err
	}

	return conn, nil
}

func Apply(conn *grpc.ClientConn, data []byte) error {
	req := ApplyRequest{
		Data: data,
	}

	s := time.Now()
	_, err := NewForwardClient(conn).Apply(context.Background(), &req)
	utils.PromSink.AddSample(rpcForwardLatency, float32(time.Since(s).Seconds()))
	return err
}

func RemoveServer(conn *grpc.ClientConn, serverId string) error {
	req := ShutdownRequest{
		ServerId: serverId,
	}
	_, err := NewForwardClient(conn).Shutdown(context.Background(), &req)
	return err
}
