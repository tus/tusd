FROM golang:1.7-alpine

# Copy in the git repo from the build context
COPY . /go/src/github.com/tus/tusd/

# Create app directory

RUN addgroup -g 1000 tusd \
    && adduser -u 1000 -G tusd -s /bin/sh -D tusd \
    && cd /go/src/github.com/tus/tusd \
    && apk add --no-cache \
        git \
    && go get -d -v ./... \
    && version="$(git tag -l --points-at HEAD)" \
    && commit=$(git log --format="%H" -n 1) \
    && GOOS=linux GOARCH=amd64 go build \
        -ldflags="-X github.com/tus/tusd/cmd/tusd/cli.VersionName=${version} -X github.com/tus/tusd/cmd/tusd/cli.GitCommit=${commit} -X 'github.com/tus/tusd/cmd/tusd/cli.BuildDate=$(date --utc)'" \
        -o "/go/bin/tusd" ./cmd/tusd/main.go \
    && mkdir -p /srv/tusd-hooks \
    && mkdir -p /srv/tusd-data \
    && chown tusd:tusd /srv/tusd-data \
    && rm -r /go/src/* \
    && apk del git

WORKDIR /srv/tusd-data
EXPOSE 1080
ENTRYPOINT ["/go/bin/tusd","--hooks-dir","/srv/tusd-hooks"]
