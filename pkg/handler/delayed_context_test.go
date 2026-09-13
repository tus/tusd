package handler

import (
	"context"
	"testing"
	"time"

	"testing/synctest"

	"github.com/stretchr/testify/assert"
)

func TestNewDelayedContext(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		parent, cancelParent := context.WithCancel(t.Context())
		delayedCtx := newDelayedContext(parent, 5*time.Second)
		cancelParent()

		time.Sleep(5*time.Second - time.Millisecond)
		synctest.Wait()
		assert.NoError(t, delayedCtx.Err())

		time.Sleep(time.Millisecond)
		synctest.Wait()
		assert.Equal(t, context.Canceled, delayedCtx.Err())
	})
}
