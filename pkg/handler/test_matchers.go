package handler

import (
	"context"
	"github.com/golang/mock/gomock"
)

type contextWithValuesMatcher struct {
	baseCtx context.Context
}

func WrapsContext(ctx context.Context) gomock.Matcher {
	return contextWithValuesMatcher{ctx}
}

func (c contextWithValuesMatcher) Matches(x interface{}) bool {
	ctx, ok := x.(contextWithValues)
	if !ok {
		return false
	}
	return ctx.Context == c.baseCtx
}

func (c contextWithValuesMatcher) String() string {
	return "wraps base context "
}
