package handler_test

import (
	"testing"

	"github.com/tus/tusd/pkg/handler"

	"github.com/golang/mock/gomock"
)

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
