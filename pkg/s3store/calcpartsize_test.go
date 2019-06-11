package s3store

import (
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

const enableTestDebugOutput = false

func assertCalculatedPartSize(store S3Store, assert *assert.Assertions, size int64) {
	optimalPartSize, err := store.calcOptimalPartSize(size)
	assert.Nil(err, "Size %d, no error should be returned.\n", size)

	// Number of parts with the same size
	equalparts := size / optimalPartSize
	// Size of the last part (or 0 if no spare part is needed)
	lastpartSize := size % optimalPartSize

	prelude := fmt.Sprintf("Size %d, %d parts of size %d, lastpart %d: ", size, equalparts, optimalPartSize, lastpartSize)

	assert.False(optimalPartSize < store.MinPartSize, prelude+"optimalPartSize < MinPartSize %d.\n", store.MinPartSize)
	assert.False(optimalPartSize > store.MaxPartSize, prelude+"optimalPartSize > MaxPartSize %d.\n", store.MaxPartSize)
	assert.False(lastpartSize == 0 && equalparts > store.MaxMultipartParts, prelude+"more parts than MaxMultipartParts %d.\n", store.MaxMultipartParts)
	assert.False(lastpartSize > 0 && equalparts > store.MaxMultipartParts-1, prelude+"more parts than MaxMultipartParts %d.\n", store.MaxMultipartParts)
	assert.False(lastpartSize > store.MaxPartSize, prelude+"lastpart > MaxPartSize %d.\n", store.MaxPartSize)
	assert.False(lastpartSize > optimalPartSize, prelude+"lastpart > optimalPartSize %d.\n", optimalPartSize)
	assert.True(size <= optimalPartSize*store.MaxMultipartParts, prelude+"upload does not fit in %d parts.\n", store.MaxMultipartParts)

	if enableTestDebugOutput {
		fmt.Printf(prelude+"does exceed MaxObjectSize: %t.\n", size > store.MaxObjectSize)
	}
}

func TestCalcOptimalPartSize(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	s3obj := NewMockS3API(mockCtrl)
	store := New("bucket", s3obj)

	// If you quickly want to override the default values in this test
	/*
		store.MinPartSize = 2
		store.MaxPartSize = 10
		store.MaxMultipartParts = 20
		store.MaxObjectSize = 200
	*/

	// sanity check
	if store.MaxObjectSize > store.MaxPartSize*store.MaxMultipartParts {
		t.Errorf("MaxObjectSize %v can never be achieved, as MaxMultipartParts %v and MaxPartSize %v only allow for an upload of %v bytes total.\n", store.MaxObjectSize, store.MaxMultipartParts, store.MaxPartSize, store.MaxMultipartParts*store.MaxPartSize)
	}

	HighestApplicablePartSize := store.MaxObjectSize / store.MaxMultipartParts
	if store.MaxObjectSize%store.MaxMultipartParts > 0 {
		HighestApplicablePartSize++
	}
	RemainderWithHighestApplicablePartSize := store.MaxObjectSize % HighestApplicablePartSize

	// some of these tests are actually duplicates, as they specify the same size
	// in bytes - two ways to describe the same thing. That is wanted, in order
	// to provide a full picture from any angle.
	testcases := []int64{
		0,
		1,
		store.MinPartSize - 1,
		store.MinPartSize,
		store.MinPartSize + 1,

		store.MinPartSize*(store.MaxMultipartParts-1) - 1,
		store.MinPartSize * (store.MaxMultipartParts - 1),
		store.MinPartSize*(store.MaxMultipartParts-1) + 1,

		store.MinPartSize*store.MaxMultipartParts - 1,
		store.MinPartSize * store.MaxMultipartParts,
		store.MinPartSize*store.MaxMultipartParts + 1,

		store.MinPartSize*(store.MaxMultipartParts+1) - 1,
		store.MinPartSize * (store.MaxMultipartParts + 1),
		store.MinPartSize*(store.MaxMultipartParts+1) + 1,

		(HighestApplicablePartSize-1)*store.MaxMultipartParts - 1,
		(HighestApplicablePartSize - 1) * store.MaxMultipartParts,
		(HighestApplicablePartSize-1)*store.MaxMultipartParts + 1,

		HighestApplicablePartSize*(store.MaxMultipartParts-1) - 1,
		HighestApplicablePartSize * (store.MaxMultipartParts - 1),
		HighestApplicablePartSize*(store.MaxMultipartParts-1) + 1,

		HighestApplicablePartSize*(store.MaxMultipartParts-1) + RemainderWithHighestApplicablePartSize - 1,
		HighestApplicablePartSize*(store.MaxMultipartParts-1) + RemainderWithHighestApplicablePartSize,
		HighestApplicablePartSize*(store.MaxMultipartParts-1) + RemainderWithHighestApplicablePartSize + 1,

		store.MaxObjectSize - 1,
		store.MaxObjectSize,
		store.MaxObjectSize + 1,

		(store.MaxObjectSize/store.MaxMultipartParts)*(store.MaxMultipartParts-1) - 1,
		(store.MaxObjectSize / store.MaxMultipartParts) * (store.MaxMultipartParts - 1),
		(store.MaxObjectSize/store.MaxMultipartParts)*(store.MaxMultipartParts-1) + 1,

		store.MaxPartSize*(store.MaxMultipartParts-1) - 1,
		store.MaxPartSize * (store.MaxMultipartParts - 1),
		store.MaxPartSize*(store.MaxMultipartParts-1) + 1,

		store.MaxPartSize*store.MaxMultipartParts - 1,
		store.MaxPartSize * store.MaxMultipartParts,
		// We cannot calculate a part size for store.MaxPartSize*store.MaxMultipartParts + 1
		// This case is tested in TestCalcOptimalPartSize_ExceedingMaxPartSize
	}

	for _, size := range testcases {
		assertCalculatedPartSize(store, assert, size)
	}

	if enableTestDebugOutput {
		fmt.Println("HighestApplicablePartSize", HighestApplicablePartSize)
		fmt.Println("RemainderWithHighestApplicablePartSize", RemainderWithHighestApplicablePartSize)
	}
}

func TestCalcOptimalPartSize_AllUploadSizes(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	s3obj := NewMockS3API(mockCtrl)
	store := New("bucket", s3obj)

	store.MinPartSize = 5
	store.MaxPartSize = 5 * 1024
	store.MaxMultipartParts = 1000
	store.MaxObjectSize = store.MaxPartSize * store.MaxMultipartParts

	// sanity check
	if store.MaxObjectSize > store.MaxPartSize*store.MaxMultipartParts {
		t.Errorf("MaxObjectSize %v can never be achieved, as MaxMultipartParts %v and MaxPartSize %v only allow for an upload of %v bytes total.\n", store.MaxObjectSize, store.MaxMultipartParts, store.MaxPartSize, store.MaxMultipartParts*store.MaxPartSize)
	}

	for size := int64(0); size <= store.MaxObjectSize; size++ {
		assertCalculatedPartSize(store, assert, size)
	}
}

func TestCalcOptimalPartSize_ExceedingMaxPartSize(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	s3obj := NewMockS3API(mockCtrl)
	store := New("bucket", s3obj)

	size := store.MaxPartSize*store.MaxMultipartParts + 1

	optimalPartSize, err := store.calcOptimalPartSize(size)
	assert.NotNil(err)
	assert.EqualError(err, fmt.Sprintf("calcOptimalPartSize: to upload %v bytes optimalPartSize %v must exceed MaxPartSize %v", size, optimalPartSize, store.MaxPartSize))
}
