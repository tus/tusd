// +build !go1.7

package tusd_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
)

var subTestDepth = 0

func SubTest(t *testing.T, name string, runTest func(*testing.T, *MockFullDataStore)) {
	subTestDepth++
	defer func() { subTestDepth-- }()
	p := strings.Repeat("\t", subTestDepth)

	fmt.Println(p, "=== RUN SubTest:", name)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	store := NewMockFullDataStore(ctrl)

	runTest(t, store)

	if t.Failed() {
		fmt.Println(p, "--- FAIL SubTest:", name)
		t.FailNow()
	} else {
		fmt.Println(p, "--- PASS SubTest:", name)
	}
}
