package s3log

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"golang.org/x/exp/slog"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/golang/mock/gomock"
)

//go:generate mockgen -destination=./s3log_mock_test.go -package=s3log github.com/tus/tusd/v2/pkg/s3store S3API

func TestLoggingS3API(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockS3API := NewMockS3API(mockCtrl)

	// Create a buffer to capture logs
	var logBuffer bytes.Buffer
	handler := slog.NewTextHandler(&logBuffer, &slog.HandlerOptions{Level: slog.LevelDebug})
	logger := slog.New(handler)

	wrapper := New(mockS3API, logger)

	ctx := context.Background()
	input := &s3.PutObjectInput{
		Bucket:  aws.String("test-bucket"),
		Key:     aws.String("test-key"),
		Body:    bytes.NewReader([]byte("body data that should not be logged")),
		Tagging: aws.String("tag1=value1&tag2=value2"),
	}
	expectedOutput := &s3.PutObjectOutput{
		ETag: aws.String("test-etag"),
	}

	mockS3API.EXPECT().
		PutObject(ctx, input, gomock.Any()).
		Return(expectedOutput, nil)

	output, err := wrapper.PutObject(ctx, input)
	if err != nil {
		t.Fatalf("PutObject failed: %v", err)
	}

	// Check output
	if *output.ETag != *expectedOutput.ETag {
		t.Errorf("Expected ETag %s, got %s", *expectedOutput.ETag, *output.ETag)
	}

	// Check logs
	logs := logBuffer.String()

	// Verify PutObject log doesn't contain the body
	if strings.Contains(logs, "body data") {
		t.Error("PutObject log should not contain the request body")
	}

	// Verify operation name is logged
	if !strings.Contains(logs, "operation=PutObject") {
		t.Error("Logs should contain the operation name")
	}

	// Verify input is logged
	if !strings.Contains(logs, "test-bucket") {
		t.Error("Logs should contain the bucket name")
	}
	if !strings.Contains(logs, "test-key") {
		t.Error("Logs should contain the key name")
	}
	if !strings.Contains(logs, "tag1=value1") {
		t.Error("Logs should contain the tags")
	}

	// Verify output is logged
	if !strings.Contains(logs, "test-etag") {
		t.Error("Logs should contain the ETag from the output")
	}
}
