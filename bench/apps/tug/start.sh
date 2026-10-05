#!/usr/bin/env bash
# Serves the app on 127.0.0.1:$PORT, in one process, which uses every core.
set -euo pipefail
# APP_KEY encrypts the session's cookie. This one was made for the
# benchmarks and guards nothing.
export APP_KEY=base64:bW8xvTqOJ4qJr5yn5I1uSj0ZxPtpbm3U2X0GoWvWyrk=
export ADDR=127.0.0.1:$PORT
exec .build/blog
