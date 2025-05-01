module github.com/tus/tusd/v2

// Specify the Go version needed for the Heroku deployment
// See https://github.com/heroku/heroku-buildpack-go#go-module-specifics
// +heroku goVersion go1.24
go 1.23.0

toolchain go1.24.1

require (
	cloud.google.com/go/storage v1.52.0
	github.com/Acconut/go-httptest-recorder v1.0.0
	github.com/Azure/azure-sdk-for-go/sdk/azcore v1.18.0
	github.com/Azure/azure-sdk-for-go/sdk/storage/azblob v1.6.1
	github.com/Shopify/toxiproxy/v2 v2.12.0
	github.com/aws/aws-sdk-go-v2 v1.36.3
	github.com/aws/aws-sdk-go-v2/config v1.29.14
	github.com/aws/aws-sdk-go-v2/service/s3 v1.79.3
	github.com/aws/smithy-go v1.22.3
	github.com/bmizerany/pat v0.0.0-20170815010413-6226ea591a40
	github.com/felixge/fgprof v0.9.5
	github.com/goji/httpauth v0.0.0-20160601135302-2da839ab0f4d
	github.com/golang/mock v1.6.0
	github.com/grpc-ecosystem/go-grpc-middleware v1.4.0
	github.com/hashicorp/go-hclog v1.6.3
	github.com/hashicorp/go-plugin v1.6.3
	github.com/prometheus/client_golang v1.22.0
	github.com/rs/zerolog v1.34.0
	github.com/sethgrid/pester v1.2.0
	github.com/stretchr/testify v1.10.0
	github.com/tus/lockfile v1.2.0
	github.com/vimeo/go-util v1.4.1
	golang.org/x/exp v0.0.0-20250106191152-7588d65b2ba8
	golang.org/x/net v0.39.0
	golang.org/x/sync v0.13.0
	google.golang.org/api v0.229.0
	google.golang.org/grpc v1.71.1
	google.golang.org/protobuf v1.36.6
	gopkg.in/h2non/gock.v1 v1.1.2
)

require (
	cel.dev/expr v0.19.2 // indirect
	cloud.google.com/go v0.120.0 // indirect
	cloud.google.com/go/auth v0.16.0 // indirect
	cloud.google.com/go/auth/oauth2adapt v0.2.8 // indirect
	cloud.google.com/go/compute/metadata v0.6.0 // indirect
	cloud.google.com/go/iam v1.5.0 // indirect
	cloud.google.com/go/monitoring v1.24.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/internal v1.11.1 // indirect
	github.com/GoogleCloudPlatform/opentelemetry-operations-go/detectors/gcp v1.27.0 // indirect
	github.com/GoogleCloudPlatform/opentelemetry-operations-go/exporter/metric v0.51.0 // indirect
	github.com/GoogleCloudPlatform/opentelemetry-operations-go/internal/resourcemapping v0.51.0 // indirect
	github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.6.10 // indirect
	github.com/aws/aws-sdk-go-v2/credentials v1.17.67 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.16.30 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.3.34 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.6.34 // indirect
	github.com/aws/aws-sdk-go-v2/internal/ini v1.8.3 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.3.34 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.12.3 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/checksum v1.7.1 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.12.15 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/s3shared v1.18.15 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.25.3 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.30.1 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.33.19 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/cncf/xds/go v0.0.0-20250121191232-2f005788dc42 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/envoyproxy/go-control-plane/envoy v1.32.4 // indirect
	github.com/envoyproxy/protoc-gen-validate v1.2.1 // indirect
	github.com/fatih/color v1.13.0 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/go-logr/logr v1.4.2 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/google/pprof v0.0.0-20240227163752-401108e1b7e7 // indirect
	github.com/google/s2a-go v0.1.9 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/googleapis/enterprise-certificate-proxy v0.3.6 // indirect
	github.com/googleapis/gax-go/v2 v2.14.1 // indirect
	github.com/gorilla/mux v1.8.1 // indirect
	github.com/h2non/parth v0.0.0-20190131123155-b4df798d6542 // indirect
	github.com/hashicorp/yamux v0.1.1 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/oklog/run v1.0.0 // indirect
	github.com/planetscale/vtprotobuf v0.6.1-0.20240319094008-0393e58bdf10 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/prometheus/client_model v0.6.1 // indirect
	github.com/prometheus/common v0.62.0 // indirect
	github.com/prometheus/procfs v0.15.1 // indirect
	github.com/rs/xid v1.6.0 // indirect
	go.opentelemetry.io/auto/sdk v1.1.0 // indirect
	go.opentelemetry.io/contrib/detectors/gcp v1.35.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.60.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.60.0 // indirect
	go.opentelemetry.io/otel v1.35.0 // indirect
	go.opentelemetry.io/otel/metric v1.35.0 // indirect
	go.opentelemetry.io/otel/sdk v1.35.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v1.35.0 // indirect
	go.opentelemetry.io/otel/trace v1.35.0 // indirect
	golang.org/x/crypto v0.37.0 // indirect
	golang.org/x/oauth2 v0.29.0 // indirect
	golang.org/x/sys v0.32.0 // indirect
	golang.org/x/text v0.24.0 // indirect
	golang.org/x/time v0.11.0 // indirect
	google.golang.org/genproto v0.0.0-20250303144028-a0af3efb3deb // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20250414145226-207652e42e2e // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250414145226-207652e42e2e // indirect
	gopkg.in/tomb.v1 v1.0.0-20141024135613-dd632973f1e7 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
