// Package grpc implements a gRPC-based hook system. For each hook event, the InvokeHook
// procedure is invoked with additional details about the hook type, upload and request.
// The Protocol Buffers are defined in github.com/tus/tusd/v2/pkg/hooks/grpc/proto/hook.proto.
package grpc

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net/http"
	"os"
	"time"

	grpc_retry "github.com/grpc-ecosystem/go-grpc-middleware/retry"
	"github.com/tus/tusd/v2/pkg/hooks"
	pb "github.com/tus/tusd/v2/pkg/hooks/grpc/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

type GrpcHook struct {
	Endpoint                        string
	MaxRetries                      int
	Backoff                         time.Duration
	Client                          pb.HookHandlerClient
	Secure                          bool
	ServerTLSCertificateFilePath    string
	ClientTLSCertificateFilePath    string
	ClientTLSCertificateKeyFilePath string
}

func (g *GrpcHook) Setup() error {
	grpcOpts := []grpc.DialOption{}

	if g.Secure {
		if g.ServerTLSCertificateFilePath == "" {
			return errors.New("hooks-grpc-secure was set to true but no gRPC server TLS certificate file was provided. A value for hooks-grpc-server-tls-certificate is missing")
		}

		// Load the server's TLS certificate if provided
		serverCert, err := os.ReadFile(g.ServerTLSCertificateFilePath)
		if err != nil {
			return err
		}

		// Create a certificate pool and add the server's certificate
		certPool := x509.NewCertPool()
		certPool.AppendCertsFromPEM(serverCert)

		// Create TLS configuration with the server's CA certificate
		tlsConfig := &tls.Config{
			RootCAs: certPool,
		}

		// If client's TLS certificate and key file paths are provided, use mutual TLS
		if g.ClientTLSCertificateFilePath != "" && g.ClientTLSCertificateKeyFilePath != "" {
			// Load the client's TLS certificate and private key
			clientCert, err := tls.LoadX509KeyPair(g.ClientTLSCertificateFilePath, g.ClientTLSCertificateKeyFilePath)
			if err != nil {
				return err
			}

			// Append client certificate to the TLS configuration
			tlsConfig.Certificates = append(tlsConfig.Certificates, clientCert)
		}

		grpcOpts = append(grpcOpts, grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)))
	} else {
		grpcOpts = append(grpcOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	opts := []grpc_retry.CallOption{
		grpc_retry.WithBackoff(grpc_retry.BackoffLinear(g.Backoff)),
		grpc_retry.WithMax(uint(g.MaxRetries)),
	}
	grpcOpts = append(grpcOpts, grpc.WithUnaryInterceptor(grpc_retry.UnaryClientInterceptor(opts...)))

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
