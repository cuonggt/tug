#!/usr/bin/env bash
# Prints what the benchmark runs: the runtime's version, and the framework's,
# its HTTP server's and the Inertia adapter's as package-lock.json pins them.
set -euo pipefail
cd "$(dirname "$0")"

locked() {
  node -p "require('./package-lock.json').packages['node_modules/$1'].version"
}

echo "Node $(node -p 'process.versions.node')"
echo "AdonisJS $(locked @adonisjs/core)"
echo "@adonisjs/http-server $(locked @adonisjs/http-server)"
echo "@adonisjs/inertia $(locked @adonisjs/inertia)"
