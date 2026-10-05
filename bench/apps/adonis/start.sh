#!/usr/bin/env bash
# Serves the production build on 127.0.0.1:$PORT in $WORKERS processes, in
# the foreground, until it's stopped by a signal to its process group.
set -euo pipefail

: "${PORT:?set PORT to the port to listen on}"

# The environment start/env.ts asks for, in production, and the starter
# kit's TZ. AdonisJS's deployment guide (docs.adonisjs.com/deployment) has
# the environment come from the platform rather than a .env file, which the
# build leaves out.
export NODE_ENV=production
export HOST=127.0.0.1
export PORT
export APP_URL="http://127.0.0.1:$PORT"
export TZ=UTC

# Warnings and errors only, to stdout through pino, as the guide says logs
# go: Adonis logs no line per request, and this also leaves out the line
# each worker writes as it starts.
export LOG_LEVEL=warn

# Sessions in encrypted cookies, as the starter kit has them, so the app
# needs no database or Redis.
export SESSION_DRIVER=cookie

# Made with `node ace generate:key` for the benchmark, and committed, as it
# guards nothing: the sessions it seals hold only shield's CSRF secret, on a
# server that listens on the loopback alone.
export APP_KEY=LqLEb0Z_4bFY-n9LneavhbEVmjSkmSka

# The guide runs the build from build/, its root in production;
# bin/cluster.js starts $WORKERS processes of its bin/server.js, one per
# CPU when WORKERS isn't set.
cd "$(dirname "$0")/build"
exec node bin/cluster.js
