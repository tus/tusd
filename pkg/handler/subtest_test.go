package handler_test

import (
	"testing"

	"github.com/golang/mock/gomock"
)

func SubTest(t *testing.T, name string, runTest func(*testing.T, *MockFullDataStore)) {
	t.Run(name, func(subT *testing.T) {
		//subT.Parallel()

		ctrl := gomock.NewController(subT)
		defer ctrl.Finish()

		store := NewMockFullDataStore(ctrl)

		runTest(subT, store)
	})
}
