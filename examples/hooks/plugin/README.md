# Example plugin hook

This directory contains an example for a [plugin-based hook handler](https://tus.github.io/tusd/advanced-topics/hooks/#plugin-hooks). Tusd supports plugins via the [go-plugin](https://github.com/hashicorp/go-plugin) package, where plugins run as standalone processes and communicate with tusd via local sockets using Go's `net/rpc` package.

## Build

To build the plugin, run `make hook_handler`. This will create a binary called `hook_handler` that is then later run by tusd.

## Run

To start tusd with the plugin, run `tusd -hooks-plugin ./hook_handler`. The output should reflect that the plugin was loaded successfully. Whenever enabled hook events are triggered, they will be handled by the plugin.
