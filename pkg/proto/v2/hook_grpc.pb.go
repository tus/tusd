// Code generated by protoc-gen-go-grpc. DO NOT EDIT.
// versions:
// - protoc-gen-go-grpc v1.2.0
// - protoc             v3.21.12
// source: cmd/tusd/cli/hooks/proto/v2/hook.proto

package v2

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

// HookHandlerClient is the client API for HookHandler service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type HookHandlerClient interface {
	// InvokeHook is invoked for every hook that is executed. HookRequest contains the
	// corresponding information about the hook type, the involved upload, and
	// causing HTTP request.
	// The return value HookResponse allows to stop or reject an upload, as well as modifying
	// the HTTP response. See the documentation for HookResponse for more details.
	InvokeHook(ctx context.Context, in *HookRequest, opts ...grpc.CallOption) (*HookResponse, error)
}

type hookHandlerClient struct {
	cc grpc.ClientConnInterface
}

func NewHookHandlerClient(cc grpc.ClientConnInterface) HookHandlerClient {
	return &hookHandlerClient{cc}
}

func (c *hookHandlerClient) InvokeHook(ctx context.Context, in *HookRequest, opts ...grpc.CallOption) (*HookResponse, error) {
	out := new(HookResponse)
	err := c.cc.Invoke(ctx, "/v2.HookHandler/InvokeHook", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// HookHandlerServer is the server API for HookHandler service.
// All implementations must embed UnimplementedHookHandlerServer
// for forward compatibility
type HookHandlerServer interface {
	// InvokeHook is invoked for every hook that is executed. HookRequest contains the
	// corresponding information about the hook type, the involved upload, and
	// causing HTTP request.
	// The return value HookResponse allows to stop or reject an upload, as well as modifying
	// the HTTP response. See the documentation for HookResponse for more details.
	InvokeHook(context.Context, *HookRequest) (*HookResponse, error)
	mustEmbedUnimplementedHookHandlerServer()
}

// UnimplementedHookHandlerServer must be embedded to have forward compatible implementations.
type UnimplementedHookHandlerServer struct {
}

func (UnimplementedHookHandlerServer) InvokeHook(context.Context, *HookRequest) (*HookResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method InvokeHook not implemented")
}
func (UnimplementedHookHandlerServer) mustEmbedUnimplementedHookHandlerServer() {}

// UnsafeHookHandlerServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to HookHandlerServer will
// result in compilation errors.
type UnsafeHookHandlerServer interface {
	mustEmbedUnimplementedHookHandlerServer()
}

func RegisterHookHandlerServer(s grpc.ServiceRegistrar, srv HookHandlerServer) {
	s.RegisterService(&HookHandler_ServiceDesc, srv)
}

func _HookHandler_InvokeHook_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(HookRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(HookHandlerServer).InvokeHook(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/v2.HookHandler/InvokeHook",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(HookHandlerServer).InvokeHook(ctx, req.(*HookRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// HookHandler_ServiceDesc is the grpc.ServiceDesc for HookHandler service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var HookHandler_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "v2.HookHandler",
	HandlerType: (*HookHandlerServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "InvokeHook",
			Handler:    _HookHandler_InvokeHook_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "cmd/tusd/cli/hooks/proto/v2/hook.proto",
}
