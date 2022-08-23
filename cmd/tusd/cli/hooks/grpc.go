package hooks

import (
	"context"
	"time"

	grpc_retry "github.com/grpc-ecosystem/go-grpc-middleware/retry"
	pb "github.com/tackboon/tusd/pkg/proto/v2"
	"github.com/tus/tusd/pkg/handler"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

type GrpcHook struct {
	Endpoint   string
	MaxRetries int
	Backoff    int
	Client     pb.HookServiceClient
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
	g.Client = pb.NewHookServiceClient(conn)
	return nil
}

func (g *GrpcHook) InvokeHook(typ HookType, info handler.HookEvent, captureOutput bool) ([]byte, int, error) {
	ctx := context.Background()
	req := &pb.SendRequest{Hook: marshal(typ, info)}
	resp, err := g.Client.Send(ctx, req)
	if err != nil {
		if e, ok := status.FromError(err); ok {
			return nil, int(e.Code()), err
		}
		return nil, 2, err
	}
	if captureOutput {
		return resp.Response.GetValue(), 0, err
	}
	return nil, 0, err
}

func marshal(typ HookType, info handler.HookEvent) *pb.Hook {
	return &pb.Hook{
		Upload: &pb.Upload{
			Id:             info.Upload.ID,
			Size:           info.Upload.Size,
			SizeIsDeferred: info.Upload.SizeIsDeferred,
			Offset:         info.Upload.Offset,
			MetaData:       info.Upload.MetaData,
			IsPartial:      info.Upload.IsPartial,
			IsFinal:        info.Upload.IsFinal,
			PartialUploads: info.Upload.PartialUploads,
			Storage:        info.Upload.Storage,
		},
		HttpRequest: &pb.HTTPRequest{
			Method:     info.HTTPRequest.Method,
			Uri:        info.HTTPRequest.URI,
			RemoteAddr: info.HTTPRequest.RemoteAddr,
		},
		Name: string(typ),
	}
}
