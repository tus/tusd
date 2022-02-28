# Examples

This directory contains following examples:

- `apache2.conf` is the recommended minimum configuration for an Apache2 proxy in front of tusd.
- `nginx.conf` is the recommended minimum configuration for an Nginx proxy in front of tusd.
- `server/` is an example of how to the tusd package embedded in your own Go application.
- `hooks/file/` are Bash scripts for file hook implementations.
- `hooks/http/` is a Python HTTP server as the HTTP hook implementation.
- `hooks/grpc/` is a Python gRPC server as the gRPC hook implementation.
- `hooks/plugin/` is a Go plugin usable with the plugin hooks.
