#!/usr/bin/env bash
set -euo pipefail
echo "Go $(go env GOVERSION | sed 's/^go//')"
echo "tug $(git describe --tags --always 2>/dev/null || echo unknown)"
