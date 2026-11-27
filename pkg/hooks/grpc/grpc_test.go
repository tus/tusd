package grpc

import (
	"context"
	"net"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tus/tusd/v2/pkg/handler"
	"github.com/tus/tusd/v2/pkg/hooks"
	pb "github.com/tus/tusd/v2/pkg/hooks/grpc/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// testHookServer is a gRPC hook server whose behavior is controlled by a function.
// It records all requests and their incoming metadata.
type testHookServer struct {
	pb.UnimplementedHookHandlerServer

	handle func(call int, req *pb.HookRequest) (*pb.HookResponse, error)

	mu       sync.Mutex
	requests []*pb.HookRequest
	metadata []metadata.MD
}

func (s *testHookServer) InvokeHook(ctx context.Context, req *pb.HookRequest) (*pb.HookResponse, error) {
	md, _ := metadata.FromIncomingContext(ctx)

	s.mu.Lock()
	s.requests = append(s.requests, req)
	s.metadata = append(s.metadata, md)
	call := len(s.requests)
	s.mu.Unlock()

	return s.handle(call, req)
}

func (s *testHookServer) calls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.requests)
}

// startTestHookServer starts a gRPC hook server on a random local port and returns
// a set up GrpcHook that talks to it. configure can adjust the GrpcHook before Setup.
func startTestHookServer(t *testing.T, srv *testHookServer, configure func(*GrpcHook)) *GrpcHook {
	t.Helper()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	server := grpc.NewServer()
	pb.RegisterHookHandlerServer(server, srv)
	go server.Serve(lis)
	t.Cleanup(server.Stop)

	hook := &GrpcHook{
		Endpoint: lis.Addr().String(),
	}
	if configure != nil {
		configure(hook)
	}
	require.NoError(t, hook.Setup())
	return hook
}

// TestFieldParity ensures that the Go hook types and the Protocol Buffers messages
// have the same fields. The gRPC backend converts between them by hand, so a field
// that is added to one side but not the other would otherwise be silently dropped.
func TestFieldParity(t *testing.T) {
	tests := []struct {
		goType  reflect.Type
		message proto.Message
		// ignored lists Go fields which are intentionally not transmitted.
		ignored []string
	}{
		{reflect.TypeFor[hooks.HookRequest](), &pb.HookRequest{}, nil},
		{reflect.TypeFor[handler.HookEvent](), &pb.Event{}, []string{"Context"}},
		{reflect.TypeFor[handler.FileInfo](), &pb.FileInfo{}, nil},
		{reflect.TypeFor[handler.HTTPRequest](), &pb.HTTPRequest{}, nil},
		{reflect.TypeFor[hooks.HookResponse](), &pb.HookResponse{}, nil},
		{reflect.TypeFor[handler.HTTPResponse](), &pb.HTTPResponse{}, nil},
		{reflect.TypeFor[handler.FileInfoChanges](), &pb.FileInfoChanges{}, nil},
	}

	for _, test := range tests {
		desc := test.message.ProtoReflect().Descriptor()
		t.Run(test.goType.String(), func(t *testing.T) {
			protoFields := map[string]bool{}
			fields := desc.Fields()
			for i := 0; i < fields.Len(); i++ {
				protoFields[strings.ToLower(string(fields.Get(i).Name()))] = true
			}

			goFields := map[string]bool{}
			for field := range test.goType.Fields() {
				if !field.IsExported() || slices.Contains(test.ignored, field.Name) {
					continue
				}
				name := strings.ToLower(field.Name)
				goFields[name] = true
				assert.True(t, protoFields[name], "Go field %s.%s has no counterpart in proto message %s", test.goType, field.Name, desc.FullName())
			}

			for name := range protoFields {
				assert.True(t, goFields[name], "proto field %s.%s has no counterpart in Go type %s", desc.FullName(), name, test.goType)
			}
		})
	}
}

// TestResponseRoundTrip sends a hook response with every field set from a gRPC server
// and checks that all values arrive in the hooks.HookResponse.
func TestResponseRoundTrip(t *testing.T) {
	a := assert.New(t)

	response := &pb.HookResponse{
		HttpResponse: &pb.HTTPResponse{
			StatusCode: 418,
			Header:     map[string]string{"X-Hello": "world"},
			Body:       "body",
		},
		RejectUpload: true,
		ChangeFileInfo: &pb.FileInfoChanges{
			Id:       "new-id",
			MetaData: map[string]string{"filename": "new.txt"},
			Storage:  map[string]string{"Path": "/somewhere"},
		},
		StopUpload:        true,
		RejectTermination: true,
	}
	assertProtoFullyPopulated(t, response.ProtoReflect())

	expected := hooks.HookResponse{
		HTTPResponse: handler.HTTPResponse{
			StatusCode: 418,
			Header:     handler.HTTPHeader{"X-Hello": "world"},
			Body:       "body",
		},
		RejectUpload: true,
		ChangeFileInfo: handler.FileInfoChanges{
			ID:       "new-id",
			MetaData: handler.MetaData{"filename": "new.txt"},
			Storage:  map[string]string{"Path": "/somewhere"},
		},
		StopUpload:        true,
		RejectTermination: true,
	}
	assertGoFullyPopulated(t, reflect.ValueOf(expected), "HookResponse")

	srv := &testHookServer{
		handle: func(int, *pb.HookRequest) (*pb.HookResponse, error) {
			return response, nil
		},
	}
	hook := startTestHookServer(t, srv, nil)

	res, err := hook.InvokeHook(hooks.HookRequest{Type: hooks.HookPreCreate})
	a.NoError(err)
	a.Equal(expected, res)
}

// TestRequestRoundTrip sends a hook request with every field set to a gRPC server
// and checks that all values arrive in the pb.HookRequest.
func TestRequestRoundTrip(t *testing.T) {
	a := assert.New(t)

	request := hooks.HookRequest{
		Type: hooks.HookPostFinish,
		Event: handler.HookEvent{
			Context: context.Background(),
			Upload: handler.FileInfo{
				ID:             "id",
				Size:           100,
				SizeIsDeferred: true,
				Offset:         42,
				MetaData:       handler.MetaData{"filename": "a.txt"},
				IsPartial:      true,
				IsFinal:        true,
				PartialUploads: []string{"a", "b"},
				Storage:        map[string]string{"Type": "filestore"},
			},
			HTTPRequest: handler.HTTPRequest{
				Method:     "PATCH",
				URI:        "/files/id",
				RemoteAddr: "127.0.0.1:1234",
				Header: http.Header{
					"X-Single": {"one"},
					// Only the first value of a header is transmitted.
					"X-Multi": {"first", "second"},
					// Headers without values are dropped.
					"X-Empty": {},
				},
			},
		},
	}
	assertGoFullyPopulated(t, reflect.ValueOf(request), "HookRequest")

	expected := &pb.HookRequest{
		Type: "post-finish",
		Event: &pb.Event{
			Upload: &pb.FileInfo{
				Id:             "id",
				Size:           100,
				SizeIsDeferred: true,
				Offset:         42,
				MetaData:       map[string]string{"filename": "a.txt"},
				IsPartial:      true,
				IsFinal:        true,
				PartialUploads: []string{"a", "b"},
				Storage:        map[string]string{"Type": "filestore"},
			},
			HttpRequest: &pb.HTTPRequest{
				Method:     "PATCH",
				Uri:        "/files/id",
				RemoteAddr: "127.0.0.1:1234",
				Header: map[string]string{
					"X-Single": "one",
					"X-Multi":  "first",
				},
			},
		},
	}
	assertProtoFullyPopulated(t, expected.ProtoReflect())

	srv := &testHookServer{
		handle: func(int, *pb.HookRequest) (*pb.HookResponse, error) {
			return &pb.HookResponse{}, nil
		},
	}
	hook := startTestHookServer(t, srv, nil)

	_, err := hook.InvokeHook(request)
	a.NoError(err)
	if a.Len(srv.requests, 1) {
		a.True(proto.Equal(expected, srv.requests[0]), "unexpected request:\n%v", srv.requests[0])
	}
}

func TestForwardHeaders(t *testing.T) {
	a := assert.New(t)

	srv := &testHookServer{
		handle: func(int, *pb.HookRequest) (*pb.HookResponse, error) {
			return &pb.HookResponse{}, nil
		},
	}
	hook := startTestHookServer(t, srv, func(g *GrpcHook) {
		g.ForwardHeaders = []string{"Authorization", "X-Missing"}
	})

	_, err := hook.InvokeHook(hooks.HookRequest{
		Type: hooks.HookPreCreate,
		Event: handler.HookEvent{
			HTTPRequest: handler.HTTPRequest{
				Header: http.Header{
					"Authorization": {"Bearer token"},
					"X-Other":       {"not forwarded"},
				},
			},
		},
	})
	a.NoError(err)

	if a.Len(srv.metadata, 1) {
		md := srv.metadata[0]
		a.Equal([]string{"Bearer token"}, md.Get("Authorization"))
		a.Empty(md.Get("X-Missing"))
		a.Empty(md.Get("X-Other"))
	}
}

func TestRetries(t *testing.T) {
	setup := func(t *testing.T, handle func(call int, req *pb.HookRequest) (*pb.HookResponse, error)) (*GrpcHook, *testHookServer) {
		srv := &testHookServer{handle: handle}
		hook := startTestHookServer(t, srv, func(g *GrpcHook) {
			// MaxRetries is the total number of attempts.
			g.MaxRetries = 3
			g.Backoff = time.Millisecond
		})
		return hook, srv
	}

	t.Run("succeeds after transient errors", func(t *testing.T) {
		hook, srv := setup(t, func(call int, _ *pb.HookRequest) (*pb.HookResponse, error) {
			if call < 3 {
				return nil, status.Error(codes.Unavailable, "try again")
			}
			return &pb.HookResponse{RejectUpload: true}, nil
		})

		res, err := hook.InvokeHook(hooks.HookRequest{Type: hooks.HookPreCreate})
		assert.NoError(t, err)
		assert.True(t, res.RejectUpload)
		assert.Equal(t, 3, srv.calls())
	})

	t.Run("fails after all attempts", func(t *testing.T) {
		hook, srv := setup(t, func(int, *pb.HookRequest) (*pb.HookResponse, error) {
			return nil, status.Error(codes.Unavailable, "down")
		})

		_, err := hook.InvokeHook(hooks.HookRequest{Type: hooks.HookPreCreate})
		assert.Equal(t, codes.Unavailable, status.Code(err))
		assert.Equal(t, 3, srv.calls())
	})

	t.Run("does not retry non-retriable errors", func(t *testing.T) {
		hook, srv := setup(t, func(int, *pb.HookRequest) (*pb.HookResponse, error) {
			return nil, status.Error(codes.Internal, "broken")
		})

		_, err := hook.InvokeHook(hooks.HookRequest{Type: hooks.HookPreCreate})
		assert.Equal(t, codes.Internal, status.Code(err))
		assert.Equal(t, 1, srv.calls())
	})
}

func TestSetupSecureValidation(t *testing.T) {
	t.Run("missing server certificate", func(t *testing.T) {
		hook := &GrpcHook{Endpoint: "127.0.0.1:0", Secure: true}
		assert.ErrorContains(t, hook.Setup(), "no gRPC server TLS certificate file was provided")
	})

	t.Run("unreadable server certificate", func(t *testing.T) {
		hook := &GrpcHook{
			Endpoint:                     "127.0.0.1:0",
			Secure:                       true,
			ServerTLSCertificateFilePath: t.TempDir() + "/missing.pem",
		}
		assert.Error(t, hook.Setup())
	})
}

// assertGoFullyPopulated fails if any exported field in v (recursively through structs)
// has its zero value. This forces test fixtures to be extended when new fields are added.
func assertGoFullyPopulated(t *testing.T, v reflect.Value, path string) {
	t.Helper()

	if v.Kind() != reflect.Struct {
		assert.False(t, v.IsZero(), "fixture field %s is not set", path)
		return
	}

	for i := 0; i < v.NumField(); i++ {
		field := v.Type().Field(i)
		if !field.IsExported() {
			continue
		}
		assertGoFullyPopulated(t, v.Field(i), path+"."+field.Name)
	}
}

// assertProtoFullyPopulated fails if any field in msg (recursively through messages)
// is not set. This forces test fixtures to be extended when new fields are added.
func assertProtoFullyPopulated(t *testing.T, msg protoreflect.Message) {
	t.Helper()

	fields := msg.Descriptor().Fields()
	for i := 0; i < fields.Len(); i++ {
		fd := fields.Get(i)
		if !msg.Has(fd) {
			assert.Fail(t, "fixture field is not set", "%s", fd.FullName())
			continue
		}
		if fd.Kind() == protoreflect.MessageKind && !fd.IsList() && !fd.IsMap() {
			assertProtoFullyPopulated(t, msg.Get(fd).Message())
		}
	}
}

// TestTimeout ensures that a hanging hook server causes InvokeHook to return an
// error once the timeout is reached, instead of blocking forever.
func TestTimeout(t *testing.T) {
	release := make(chan struct{})
	defer close(release)

	srv := &testHookServer{handle: func(int, *pb.HookRequest) (*pb.HookResponse, error) {
		<-release
		return &pb.HookResponse{}, nil
	}}
	hook := startTestHookServer(t, srv, func(h *GrpcHook) {
		h.Timeout = 100 * time.Millisecond
	})

	start := time.Now()
	_, err := hook.InvokeHook(hooks.HookRequest{Type: hooks.HookPreCreate})
	assert.Equal(t, codes.DeadlineExceeded, status.Code(err))
	assert.Less(t, time.Since(start), 5*time.Second)
}

// TestTimeoutRequestContext ensures that the hook invocation is cancelled when
// the context of the hook event is cancelled.
func TestTimeoutRequestContext(t *testing.T) {
	release := make(chan struct{})
	defer close(release)

	srv := &testHookServer{handle: func(int, *pb.HookRequest) (*pb.HookResponse, error) {
		<-release
		return &pb.HookResponse{}, nil
	}}
	hook := startTestHookServer(t, srv, func(h *GrpcHook) {
		h.Timeout = time.Minute
	})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := hook.InvokeHook(hooks.HookRequest{
		Type:  hooks.HookPreCreate,
		Event: handler.HookEvent{Context: ctx},
	})
	assert.Equal(t, codes.DeadlineExceeded, status.Code(err))
	assert.Less(t, time.Since(start), 5*time.Second)
}
