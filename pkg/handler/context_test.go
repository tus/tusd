package handler_test

import (
	"context"
	"reflect"
	"testing"

	. "github.com/tus/tusd/pkg/handler"
)

type testCtxKey string

func TestRequestContext(t *testing.T) {

	rctx := context.WithValue(context.Background(), testCtxKey("foo"), "bar")
	ctx := context.Background()

	wrapped := SetRequestContext(ctx, rctx)

	tctx := RequestContext(wrapped)
	if tctx == nil {
		t.Error("should return rctx")
	}

	if !reflect.DeepEqual(rctx, tctx) {
		t.Error("did not return rctx")
	}

	v := tctx.Value(testCtxKey("foo"))
	if s, ok := v.(string); !ok {
		t.Errorf("context value not string; got: %T", v)
	} else if s != "bar" {
		t.Errorf("context value somehow changed; got: %s", s)
	}
}
