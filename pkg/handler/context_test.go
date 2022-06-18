package handler

import (
	"context"
	"errors"
	"github.com/stretchr/testify/assert"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestContext(t *testing.T) {

	t.Run("new context returns values from parent context", func(t *testing.T) {
		parentCtx := context.WithValue(context.Background(), "test", "value")
		req := http.Request{}
		reqWithCtx := req.WithContext(parentCtx)
		ctx := newContext(&httptest.ResponseRecorder{}, reqWithCtx)

		ctxToTest := context.WithValue(ctx, "another", "testvalue")

		a := assert.New(t)

		a.Equal("testvalue", ctxToTest.Value("another"))
		a.Equal("value", ctxToTest.Value("test"))
	})

	t.Run("parent context cancellation does not cancel the httpContext", func(t *testing.T) {
		parentCtx := context.Background()
		req := http.Request{}
		reqWithCtx := req.WithContext(parentCtx)
		ctx := newContext(&httptest.ResponseRecorder{}, reqWithCtx)

		parentCtx.Done()

		a := assert.New(t)

		a.False(errors.Is(ctx.Err(), context.Canceled))
	})

}
