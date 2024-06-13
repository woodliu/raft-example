package rpc

import (
	"context"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/recovery"
	"github.com/hashicorp/raft"
	"github.com/woodliu/raft-example/src/utils"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"net"
	"runtime/debug"

	"google.golang.org/grpc"
)

type Server struct {
	address  string
	raftNode *raft.Raft
	logger   *zap.Logger

	UnsafeForwardServer
}

func NewRpcServer(loggerCtx context.Context, address string, raftNode *raft.Raft) *Server {
	return &Server{
		address:  address,
		raftNode: raftNode,
		logger:   utils.LoggerFromContext(loggerCtx),
	}
}

func (s *Server) StartRpc(stopCh chan struct{}) {
	listener, err := net.Listen("tcp", s.address)
	if err != nil {
		s.logger.Sugar().Fatalf("net.Listen err: %v", err)
	}

	grpcPanicRecoveryHandler := func(p any) (err error) {
		s.logger.Sugar().Infoln("msg", "recovered from panic", "panic", p, "stack", debug.Stack())
		return status.Errorf(codes.Internal, "%s", p)
	}

	grpcServer := grpc.NewServer(
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
		grpc.ChainUnaryInterceptor(
			logging.UnaryServerInterceptor(interceptorLogger(s.logger), loggingOpts...),
			recovery.UnaryServerInterceptor(recovery.WithRecoveryHandler(grpcPanicRecoveryHandler)),
		),
	)
	RegisterForwardServer(grpcServer, s)

	go func() {
		for {
			select {
			case <-stopCh:
				grpcServer.GracefulStop()
			}
		}
	}()

	err = grpcServer.Serve(listener)
	if err != nil {
		s.logger.Sugar().Fatalf("grpcServer.Serve err: %v", err)
	}
}

func (s *Server) Apply(ctx context.Context, req *ApplyRequest) (*ApplyResponse, error) {
	err := s.raftNode.Apply(req.Data, utils.EnqueueLimit).Error()
	if nil != err {
		st, _ := status.New(codes.Aborted, "raft apply failed").WithDetails(&ApplyResponse{
			Code:    -1,
			Message: err.Error(),
		})
		return nil, st.Err()
	}

	return &ApplyResponse{}, nil
}

func (s *Server) Shutdown(ctx context.Context, req *ShutdownRequest) (*ShutdownResponse, error) {
	future := s.raftNode.RemoveServer(raft.ServerID(req.ServerId), 0, 0)
	if err := future.Error(); err != nil {
		st, _ := status.New(codes.Aborted, "failed to remove raft server").WithDetails(&ApplyResponse{
			Code:    -1,
			Message: err.Error(),
		})
		return nil, st.Err()
	}
	return &ShutdownResponse{}, nil
}
