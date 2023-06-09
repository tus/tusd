module github.com/tus/tusd/v2

// Specify the Go version needed for the Heroku deployment
// See https://github.com/heroku/heroku-buildpack-go#go-module-specifics
// +heroku goVersion go1.19
go 1.16

require (
	cloud.google.com/go/storage v1.29.0
	github.com/Azure/azure-storage-blob-go v0.14.0
	github.com/aws/aws-sdk-go v1.44.211
	github.com/bmizerany/pat v0.0.0-20170815010413-6226ea591a40
	github.com/felixge/fgprof v0.9.2
	github.com/goji/httpauth v0.0.0-20160601135302-2da839ab0f4d
	github.com/golang/mock v1.6.0
	github.com/grpc-ecosystem/go-grpc-middleware v1.3.0
	github.com/hashicorp/go-hclog v0.14.1
	github.com/hashicorp/go-plugin v1.4.3
	github.com/minio/minio-go/v7 v7.0.31
	github.com/prometheus/client_golang v1.14.0
	github.com/sethgrid/pester v1.2.0
	github.com/stretchr/testify v1.8.2
	github.com/vimeo/go-util v1.4.1
	google.golang.org/api v0.111.0
	google.golang.org/grpc v1.53.0
	google.golang.org/protobuf v1.28.1
	gopkg.in/Acconut/lockfile.v1 v1.1.0
	gopkg.in/h2non/gock.v1 v1.1.2
)
