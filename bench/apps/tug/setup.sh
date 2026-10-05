#!/usr/bin/env bash
# Builds the app as tug build does: static, without its paths, and
# stripped. There's no frontend to build into it.
set -euo pipefail
CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o .build/blog .
