// Package grpc implements a gRPC-based hook system. For each hook event, the InvokeHook
// procedure is invoked with additional details about the hook type, upload and request.
// The Protocol Buffers are defined in github.com/tus/tusd/v2/pkg/hooks/grpc/proto/hook.proto.
package grpc

import (
	"context"
	"net/http"
	"time"

	"github.com/Nealsoni00/tusd/v2/pkg/hooks"
	pb "github.com/Nealsoni00/tusd/v2/pkg/hooks/grpc/proto"
	grpc_retry "github.com/grpc-ecosystem/go-grpc-middleware/retry"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type GrpcHook struct {
	Endpoint   string
	MaxRetries int
	Backoff    time.Duration
	Client     pb.HookHandlerClient
}

func (g *GrpcHook) Setup() error {
	opts := []grpc_retry.CallOption{
		grpc_retry.WithBackoff(grpc_retry.BackoffLinear(g.Backoff)),
		grpc_retry.WithMax(uint(g.MaxRetries)),
	}
	grpcOpts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(grpc_retry.UnaryClientInterceptor(opts...)),
	}
	conn, err := grpc.Dial(g.Endpoint, grpcOpts...)
	if err != nil {
		return err
	}
	g.Client = pb.NewHookHandlerClient(conn)
	return nil
}

func (g *GrpcHook) InvokeHook(hookReq hooks.HookRequest) (hookRes hooks.HookResponse, err error) {
	ctx := context.Background()
	req := marshal(hookReq)
	res, err := g.Client.InvokeHook(ctx, req)
	if err != nil {
		return hookRes, err
	}

	hookRes = unmarshal(res)
	return hookRes, nil
}

func marshal(hookReq hooks.HookRequest) *pb.HookRequest {
	event := hookReq.Event

	return &pb.HookRequest{
		Type: string(hookReq.Type),
		Event: &pb.Event{
			Upload: &pb.FileInfo{
				Id:             event.Upload.ID,
				Size:           event.Upload.Size,
				SizeIsDeferred: event.Upload.SizeIsDeferred,
				Offset:         event.Upload.Offset,
				MetaData:       event.Upload.MetaData,
				IsPartial:      event.Upload.IsPartial,
				IsFinal:        event.Upload.IsFinal,
				PartialUploads: event.Upload.PartialUploads,
				Storage:        event.Upload.Storage,
			},
			HttpRequest: &pb.HTTPRequest{
				Method:     event.HTTPRequest.Method,
				Uri:        event.HTTPRequest.URI,
				RemoteAddr: event.HTTPRequest.RemoteAddr,
				Header:     getHeader(event.HTTPRequest.Header),
			},
		},
	}
}

func getHeader(httpHeader http.Header) (hookHeader map[string]string) {
	hookHeader = make(map[string]string)
	for key, val := range httpHeader {
		if key != "" && val != nil && len(val) > 0 {
			hookHeader[key] = val[0]
		}
	}
	return hookHeader
}

func unmarshal(res *pb.HookResponse) (hookRes hooks.HookResponse) {
	hookRes.RejectUpload = res.RejectUpload
	hookRes.StopUpload = res.StopUpload

	httpRes := res.HttpResponse
	if httpRes != nil {
		hookRes.HTTPResponse.StatusCode = int(httpRes.StatusCode)
		hookRes.HTTPResponse.Header = httpRes.Header
		hookRes.HTTPResponse.Body = httpRes.Body
	}

	changes := res.ChangeFileInfo
	if changes != nil {
		hookRes.ChangeFileInfo.ID = changes.Id
		hookRes.ChangeFileInfo.MetaData = changes.MetaData
		hookRes.ChangeFileInfo.Storage = changes.Storage
	}

	return hookRes
}
