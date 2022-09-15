package hooks

import (
	"context"
	"net/http"
	"time"

	grpc_retry "github.com/grpc-ecosystem/go-grpc-middleware/retry"
	pb "github.com/tus/tusd/pkg/proto/v2"
	"google.golang.org/grpc"
)

type GrpcHook struct {
	Endpoint   string
	MaxRetries int
	Backoff    int
	Client     pb.HookHandlerClient
}

func (g *GrpcHook) Setup() error {
	opts := []grpc_retry.CallOption{
		grpc_retry.WithBackoff(grpc_retry.BackoffLinear(time.Duration(g.Backoff) * time.Second)),
		grpc_retry.WithMax(uint(g.MaxRetries)),
	}
	grpcOpts := []grpc.DialOption{
		grpc.WithInsecure(),
		grpc.WithUnaryInterceptor(grpc_retry.UnaryClientInterceptor(opts...)),
	}
	conn, err := grpc.Dial(g.Endpoint, grpcOpts...)
	if err != nil {
		return err
	}
	g.Client = pb.NewHookHandlerClient(conn)
	return nil
}

func (g *GrpcHook) InvokeHook(hookReq HookRequest) (hookRes HookResponse, err error) {
	ctx := context.Background()
	req := marshal(hookReq)
	res, err := g.Client.InvokeHook(ctx, req)
	if err != nil {
		return hookRes, err
	}

	hookRes = unmarshal(res)
	return hookRes, nil
}

func marshal(hookReq HookRequest) *pb.HookRequest {
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
				Header:     getHeaders(event.HTTPRequest.Header),
			},
		},
	}
}

func getHeaders(httpHeader http.Header) (hookHeader map[string]string) {
	hookHeader = make(map[string]string)
	for key, val := range httpHeader {
		if key != "" && val != nil && len(val) > 0 {
			hookHeader[key] = val[0]
		}
	}
	return hookHeader
}

func unmarshal(res *pb.HookResponse) (hookRes HookResponse) {
	hookRes.RejectUpload = res.RejectUpload
	hookRes.StopUpload = res.StopUpload

	httpRes := res.HttpResponse
	if httpRes != nil {
		hookRes.HTTPResponse.StatusCode = int(httpRes.StatusCode)
		hookRes.HTTPResponse.Headers = httpRes.Headers
		hookRes.HTTPResponse.Body = httpRes.Body
	}

	return hookRes
}
