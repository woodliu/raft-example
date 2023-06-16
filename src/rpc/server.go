package rpc

import (
	"context"
	"github.com/hashicorp/raft"
	raft2 "github.com/woodliu/raft-example/src/raft"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"net"

	"google.golang.org/grpc"
)

type Server struct {
	address  string
	raftNode *raft.Raft

	UnsafeForwardServer
}

func NewRpcServer(address string, raftNode *raft.Raft) *Server {
	return &Server{
		address:  address,
		raftNode: raftNode,
	}
}

func (s *Server) StartRpc() {
	listener, err := net.Listen("tcp", s.address)
	if err != nil {
		logger.Fatal().Stack().Msgf("net.Listen err: %v", err)
	}
	grpcServer := grpc.NewServer()
	RegisterForwardServer(grpcServer, s)

	err = grpcServer.Serve(listener)
	if err != nil {
		logger.Fatal().Stack().Msgf("grpcServer.Serve err: %v", err)
	}
}

func (s *Server) Route(ctx context.Context, req *RequestForward) (*RequestForwardResponse, error) {
	err := s.raftNode.Apply(req.Data, raft2.EnqueueLimit).Error()
	if nil != err {
		st, _ := status.New(codes.Aborted, "raft apply failed").WithDetails(&RequestForwardResponse{
			Code:    -1,
			Message: err.Error(),
		})
		return nil, st.Err()
	}

	return &RequestForwardResponse{}, nil
}
