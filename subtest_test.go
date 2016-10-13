// +build !go1.7

package tusd_test

import (
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
)

func SubTest(t *testing.T, name string, runTest func(*testing.T, *MockFullDataStore)) {
	fmt.Println("\t=== RUN SubTest:", name)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	store := NewMockFullDataStore(ctrl)

	runTest(t, store)

	if t.Failed() {
		fmt.Println("\t--- FAIL SubTest:", name)
		t.FailNow()
	} else {
		fmt.Println("\t--- PASS SubTest:", name)
	}
}
