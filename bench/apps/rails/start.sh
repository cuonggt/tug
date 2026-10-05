#!/usr/bin/env bash
# Runs the app in production on 127.0.0.1:$PORT, with Puma in cluster mode,
# $WORKERS workers, in the foreground. Run setup.sh first.
set -euo pipefail
cd "$(dirname "$0")"

: "${PORT:?PORT must be set}"
: "${WORKERS:?WORKERS must be set}"

export RAILS_ENV=production

# Made for the benchmark, so it's fixed and no secret: it signs and encrypts
# the session cookie of an app that guards nothing.
export SECRET_KEY_BASE=4a63decdeb1246f6084f836dce1ef43aa66518a361fcec73151bf8499eb90649eed443dde2f3051d2b8d51a3d081047ba805d07b44dbc2b846f6ba0ee1b7b9ef

# No log line per request; errors are still logged, to stdout.
export RAILS_LOG_LEVEL=warn

# Workers: one per CPU core, as Puma's deployment guide recommends
# (docs/deployment.md, "Cluster Mode Tips"). config/puma.rb, as `rails new`
# wrote it, leaves the count to WEB_CONCURRENCY, which Puma reads itself.
# With more than one worker Puma preloads the app before it forks them, by
# its default (the DSL's preload_app!), so they share the booted app.
export WEB_CONCURRENCY="$WORKERS"

# Threads: Rails' default, 3 a worker, which config/puma.rb reads from
# RAILS_MAX_THREADS; set here so the environment this runs in can't change it.
export RAILS_MAX_THREADS=3

# YJIT needs nothing here: Rails turns it on as the app boots in production
# (config.yjit, from config.load_defaults 8.1).

# Started as the Dockerfile `rails new` writes starts it, bin/rails server,
# which runs Puma with config/puma.rb. The binding keeps it on 127.0.0.1,
# where puma.rb's `port` alone would listen on every interface.
exec bin/rails server --binding 127.0.0.1 --port "$PORT"
