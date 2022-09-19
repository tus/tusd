FROM golang:1.19.1-alpine AS builder
WORKDIR /go/src/github.com/tus/tusd

# Add gcc and libc-dev early so it is cached
RUN set -xe \
	&& apk add --no-cache gcc libc-dev

# Install dependencies earlier so they are cached between builds
COPY go.mod go.sum ./
RUN set -xe \
	&& go mod download

# Copy the source code, because directories are special, there are separate layers
COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY pkg/ ./pkg/

# Get the version name and git commit as a build argument
ARG GIT_VERSION
ARG GIT_COMMIT

RUN set -xe \
	&& GOOS=linux GOARCH=amd64 go build \
        -ldflags="-X github.com/tus/tusd/cmd/tusd/cli.VersionName=${GIT_VERSION} -X github.com/tus/tusd/cmd/tusd/cli.GitCommit=${GIT_COMMIT} -X 'github.com/tus/tusd/cmd/tusd/cli.BuildDate=$(date --utc)'" \
        -o /go/bin/tusd ./cmd/tusd/main.go

# start a new stage that copies in the binary built in the previous stage
FROM alpine:3.16.2
WORKDIR /srv/tusd-data

COPY ./docker/entrypoint.sh /usr/local/share/docker-entrypoint.sh
COPY ./docker/load-env.sh /usr/local/share/load-env.sh

RUN apk add --no-cache ca-certificates jq bash \
    && addgroup -g 1000 tusd \
    && adduser -u 1000 -G tusd -s /bin/sh -D tusd \
    && mkdir -p /srv/tusd-hooks \
    && chown tusd:tusd /srv/tusd-data \
    && chmod +x /usr/local/share/docker-entrypoint.sh /usr/local/share/load-env.sh

COPY --from=builder /go/bin/tusd /usr/local/bin/tusd

EXPOSE 1080
USER tusd

ENTRYPOINT ["/usr/local/share/docker-entrypoint.sh"]
CMD [ "--hooks-dir", "/srv/tusd-hooks" ]
