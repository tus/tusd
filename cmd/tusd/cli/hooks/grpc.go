package hooks

import (
	"context"

	"github.com/golang/protobuf/proto"
	"github.com/tus/tusd/pkg/handler"
	pb "github.com/tus/tusd/pkg/proto/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

type GrpcHook struct {
	Endpoint string
}

func (_ GrpcHook) Setup() error {
	return nil
}

func (g GrpcHook) InvokeHook(typ HookType, info handler.HookEvent, captureOutput bool) ([]byte, int, error) {
	ctx := context.Background()

	conn, err := grpc.Dial(g.Endpoint, grpc.WithInsecure())
	if err != nil {
		return nil, 0, err
	}
	defer conn.Close()
	hc := pb.NewHookServiceClient(conn)

	req := &pb.SendRequest{Hook: marshal(info)}
	resp, err := hc.Send(ctx, req)
	if err != nil {
		if e, ok := status.FromError(err); ok {
			return nil, int(e.Code()), err
		}
		return nil, 0, err
	}
	data, err := proto.Marshal(resp)
	if err != nil {
		return nil, 0, err
	}
	return data, int(resp.StatusCode), err
}

func marshal(info handler.HookEvent) *pb.Hook {
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
	}
}
