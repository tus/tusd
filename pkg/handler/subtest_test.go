package handler_test

import (
	"testing"

	"github.com/tus/tusd/v2/pkg/handler"
	"github.com/tus/tusd/v2/pkg/memorylocker"

	"github.com/golang/mock/gomock"
)

// newTestHandler installs an in-memory locker when a test did not set one.
// Production code requires a locker, but most handler tests exercise the
// protocol rather than locking itself.
func newTestHandler(config handler.Config) (*handler.Handler, error) {
	ensureTestLocker(config.StoreComposer)
	return handler.NewHandler(config)
}

func newTestUnroutedHandler(config handler.Config) (*handler.UnroutedHandler, error) {
	ensureTestLocker(config.StoreComposer)
	return handler.NewUnroutedHandler(config)
}

func ensureTestLocker(composer *handler.StoreComposer) {
	if composer != nil && !composer.UsesLocker {
		memorylocker.New().UseIn(composer)
	}
}

func SubTest(t *testing.T, name string, runTest func(*testing.T, *MockFullDataStore, *handler.StoreComposer)) {
	t.Run(name, func(subT *testing.T) {
		//subT.Parallel()

		ctrl := gomock.NewController(subT)
		defer ctrl.Finish()

		store := NewMockFullDataStore(ctrl)
		composer := handler.NewStoreComposer()
		composer.UseCore(store)
		composer.UseTerminater(store)
		composer.UseConcater(store)
		composer.UseLengthDeferrer(store)

		runTest(subT, store, composer)
	})
}
