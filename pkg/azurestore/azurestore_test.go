package azurestore

import "github.com/tus/tusd/pkg/handler"

//go:generate mockgen -destination=./s3store_mock_test.go -package=s3store github.com/tus/tusd/pkg/s3store S3API

// Test interface implementations
var _ handler.DataStore = AzureStore{}
var _ handler.TerminaterDataStore = AzureStore{}
var _ handler.LengthDeferrerDataStore = AzureStore{}
