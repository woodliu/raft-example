package rpc

import (
	"context"
	"github.com/hashicorp/raft"
	raft2 "github.com/woodliu/raft-example/src/raft"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"log"
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
		log.Fatalf("net.Listen err: %v", err)
	}
	grpcServer := grpc.NewServer()
	RegisterForwardServer(grpcServer, &Server{})

	err = grpcServer.Serve(listener)
	if err != nil {
		log.Fatalf("grpcServer.Serve err: %v", err)
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

	st, _ := status.New(codes.OK, "raft apply successfully").WithDetails(&RequestForwardResponse{
		Code:    0,
		Message: "ok",
	})
	return nil, st.Err()
}
