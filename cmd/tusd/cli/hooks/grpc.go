package hooks

import (
	"context"
	"io"
	"log"
	"time"

	"github.com/golang/protobuf/proto"
	grpc_retry "github.com/grpc-ecosystem/go-grpc-middleware/retry"
	"github.com/tus/tusd/pkg/handler"
	pb "github.com/tus/tusd/pkg/proto/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

type GrpcHook struct {
	Endpoint   string
	MaxRetries int
	Backoff    int
}

func (_ GrpcHook) Setup() error {
	return nil
}

func (g GrpcHook) InvokeHook(typ HookType, info handler.HookEvent, captureOutput bool) ([]byte, int, error) {
	ctx := context.Background()
	opts := []grpc_retry.CallOption{
		grpc_retry.WithBackoff(grpc_retry.BackoffLinear(time.Duration(g.Backoff) * time.Second)),
		grpc_retry.WithMax(uint(g.MaxRetries)),
	}
	conn, err := grpc.Dial(g.Endpoint,
		grpc.WithInsecure(),
		grpc.WithStreamInterceptor(grpc_retry.StreamClientInterceptor(opts...)),
		grpc.WithUnaryInterceptor(grpc_retry.UnaryClientInterceptor(opts...)),
	)
	if err != nil {
		return nil, 0, err
	}
	defer conn.Close()
	client := pb.NewHookServiceClient(conn)
	stream, err := client.Send(ctx)
	if err != nil {
		if e, ok := status.FromError(err); ok {
			return nil, int(e.Code()), err
		}
		return nil, 0, err
	}

	var data []byte
	var resp *pb.SendResponse
	ctx = stream.Context()
	done := make(chan bool)

	req := &pb.SendRequest{Hook: marshal(info)}
	if err := stream.Send(req); err != nil {
		return nil, 0, err
	}
	if err := stream.CloseSend(); err != nil {
		return nil, 0, err
	}
	go func() {
		for {
			resp, err := stream.Recv()
			if err == io.EOF {
				close(done)
				return
			}
			if err != nil {
				log.Fatalf("can not receive %v", err)
			}
			data, err := proto.Marshal(resp)
			if err != nil {
				log.Fatalf("can not marshal response %v", err)
			}
			log.Printf("new response %d received", data)
		}
	}()
	go func() {
		<-ctx.Done()
		if err := ctx.Err(); err != nil {
			log.Println(err)
		}
		close(done)
	}()

	<-done
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
