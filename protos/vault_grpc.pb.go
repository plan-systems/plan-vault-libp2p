// Code generated by protoc-gen-go-grpc. DO NOT EDIT.

package protos

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

// VaultGrpcClient is the client API for VaultGrpc service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type VaultGrpcClient interface {
	// VaultSession offers a client service to a vault's feed repo.
	// The client submits requests to be executed, and the vault streams response msgs.
	// The session stream remains open as long as the client maintains its open.
	VaultSession(ctx context.Context, opts ...grpc.CallOption) (VaultGrpc_VaultSessionClient, error)
}

type vaultGrpcClient struct {
	cc grpc.ClientConnInterface
}

func NewVaultGrpcClient(cc grpc.ClientConnInterface) VaultGrpcClient {
	return &vaultGrpcClient{cc}
}

func (c *vaultGrpcClient) VaultSession(ctx context.Context, opts ...grpc.CallOption) (VaultGrpc_VaultSessionClient, error) {
	stream, err := c.cc.NewStream(ctx, &VaultGrpc_ServiceDesc.Streams[0], "/vault.VaultGrpc/VaultSession", opts...)
	if err != nil {
		return nil, err
	}
	x := &vaultGrpcVaultSessionClient{stream}
	return x, nil
}

type VaultGrpc_VaultSessionClient interface {
	Send(*FeedReq) error
	Recv() (*Msg, error)
	grpc.ClientStream
}

type vaultGrpcVaultSessionClient struct {
	grpc.ClientStream
}

func (x *vaultGrpcVaultSessionClient) Send(m *FeedReq) error {
	return x.ClientStream.SendMsg(m)
}

func (x *vaultGrpcVaultSessionClient) Recv() (*Msg, error) {
	m := new(Msg)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

// VaultGrpcServer is the server API for VaultGrpc service.
// All implementations must embed UnimplementedVaultGrpcServer
// for forward compatibility
type VaultGrpcServer interface {
	// VaultSession offers a client service to a vault's feed repo.
	// The client submits requests to be executed, and the vault streams response msgs.
	// The session stream remains open as long as the client maintains its open.
	VaultSession(VaultGrpc_VaultSessionServer) error
	mustEmbedUnimplementedVaultGrpcServer()
}

// UnimplementedVaultGrpcServer must be embedded to have forward compatible implementations.
type UnimplementedVaultGrpcServer struct {
}

func (UnimplementedVaultGrpcServer) VaultSession(VaultGrpc_VaultSessionServer) error {
	return status.Errorf(codes.Unimplemented, "method VaultSession not implemented")
}
func (UnimplementedVaultGrpcServer) mustEmbedUnimplementedVaultGrpcServer() {}

// UnsafeVaultGrpcServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to VaultGrpcServer will
// result in compilation errors.
type UnsafeVaultGrpcServer interface {
	mustEmbedUnimplementedVaultGrpcServer()
}

func RegisterVaultGrpcServer(s grpc.ServiceRegistrar, srv VaultGrpcServer) {
	s.RegisterService(&VaultGrpc_ServiceDesc, srv)
}

func _VaultGrpc_VaultSession_Handler(srv interface{}, stream grpc.ServerStream) error {
	return srv.(VaultGrpcServer).VaultSession(&vaultGrpcVaultSessionServer{stream})
}

type VaultGrpc_VaultSessionServer interface {
	Send(*Msg) error
	Recv() (*FeedReq, error)
	grpc.ServerStream
}

type vaultGrpcVaultSessionServer struct {
	grpc.ServerStream
}

func (x *vaultGrpcVaultSessionServer) Send(m *Msg) error {
	return x.ServerStream.SendMsg(m)
}

func (x *vaultGrpcVaultSessionServer) Recv() (*FeedReq, error) {
	m := new(FeedReq)
	if err := x.ServerStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

// VaultGrpc_ServiceDesc is the grpc.ServiceDesc for VaultGrpc service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var VaultGrpc_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "vault.VaultGrpc",
	HandlerType: (*VaultGrpcServer)(nil),
	Methods:     []grpc.MethodDesc{},
	Streams: []grpc.StreamDesc{
		{
			StreamName:    "VaultSession",
			Handler:       _VaultGrpc_VaultSession_Handler,
			ServerStreams: true,
			ClientStreams: true,
		},
	},
	Metadata: "protos/vault.proto",
}
