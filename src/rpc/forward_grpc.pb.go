// Code generated by protoc-gen-go-grpc. DO NOT EDIT.
// versions:
// - protoc-gen-go-grpc v1.3.0
// - protoc             v3.21.9
// source: forward.proto

package rpc

import (
	context "context"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
)

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
// Requires gRPC-Go v1.32.0 or later.
const _ = grpc.SupportPackageIsVersion7

const (
	Forward_Route_FullMethodName = "/proto.Forward/Route"
)

// ForwardClient is the client API for Forward service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type ForwardClient interface {
	Route(ctx context.Context, in *RequestForward, opts ...grpc.CallOption) (*RequestForwardResponse, error)
}

type forwardClient struct {
	cc grpc.ClientConnInterface
}

func NewForwardClient(cc grpc.ClientConnInterface) ForwardClient {
	return &forwardClient{cc}
}

func (c *forwardClient) Route(ctx context.Context, in *RequestForward, opts ...grpc.CallOption) (*RequestForwardResponse, error) {
	out := new(RequestForwardResponse)
	err := c.cc.Invoke(ctx, Forward_Route_FullMethodName, in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ForwardServer is the server API for Forward service.
// All implementations must embed UnimplementedForwardServer
// for forward compatibility
type ForwardServer interface {
	Route(context.Context, *RequestForward) (*RequestForwardResponse, error)
	mustEmbedUnimplementedForwardServer()
}

// UnimplementedForwardServer must be embedded to have forward compatible implementations.
type UnimplementedForwardServer struct {
}

func (UnimplementedForwardServer) Route(context.Context, *RequestForward) (*RequestForwardResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Route not implemented")
}
func (UnimplementedForwardServer) mustEmbedUnimplementedForwardServer() {}

// UnsafeForwardServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to ForwardServer will
// result in compilation errors.
type UnsafeForwardServer interface {
	mustEmbedUnimplementedForwardServer()
}

func RegisterForwardServer(s grpc.ServiceRegistrar, srv ForwardServer) {
	s.RegisterService(&Forward_ServiceDesc, srv)
}

func _Forward_Route_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(RequestForward)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ForwardServer).Route(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Forward_Route_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ForwardServer).Route(ctx, req.(*RequestForward))
	}
	return interceptor(ctx, in, info, handler)
}

// Forward_ServiceDesc is the grpc.ServiceDesc for Forward service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var Forward_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "proto.Forward",
	HandlerType: (*ForwardServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "Route",
			Handler:    _Forward_Route_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "forward.proto",
}
