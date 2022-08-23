.PHONY: proto
proto:
	@protoc --plugin=grpc \
		--go_out=pkg/proto \
		--go-grpc_out=pkg/proto \
		cmd/tusd/cli/hooks/proto/v2/hook.proto

.PHONY: build
build:
	@go build -o tusd cmd/tusd/main.go