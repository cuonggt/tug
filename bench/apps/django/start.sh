#!/usr/bin/env bash
# Serves the app with gunicorn on 127.0.0.1:$PORT, in the foreground. It
# execs, so gunicorn and its workers are in the runner's process group.
set -euo pipefail
cd "$(dirname "$0")"

: "${PORT:?PORT, the port to listen on, is required}"
: "${WORKERS:?WORKERS, the CPU count, is required}"

# Made for the benchmark, and fixed here, as it guards nothing: the page has
# no accounts, forms or data, and never writes the session it would sign.
export SECRET_KEY='wNhf6tvax6VcadQqQdWyJgBxqfV8orSGC4Pcbblg0negoCRZY0sPGKbck0vVVeNKbq0'

# blog.wsgi, the module, as Django's "How to use Django with Gunicorn" has it:
# https://docs.djangoproject.com/en/6.1/howto/deployment/wsgi/gunicorn/
#
# The rest by gunicorn's docs:
# --bind: 127.0.0.1 alone, as with PORT set gunicorn would listen on every
#   interface, 0.0.0.0 (https://gunicorn.org/reference/settings/#bind).
# --workers: (2 x $WORKERS) + 1, the formula of the design doc's "How Many
#   Workers?" (https://gunicorn.org/design/#how-many-workers).
# --worker-class sync: gunicorn's default, named so it's plain to see
#   (https://gunicorn.org/reference/settings/#worker_class). A worker takes
#   one request at a time and closes the connection after the response, with
#   no keep-alive, as the design doc's "Worker Types" says.
# --no-control-socket: the socket is for gunicornc, which the runner doesn't
#   use, and gunicorn would make it in ~/.gunicorn, outside the app, replacing
#   any other gunicorn's there. It's the arbiter's, not the workers', so it
#   has no part in a request
#   (https://gunicorn.org/reference/settings/#control_socket_disable).
# The access log stays off, its default, so there's no line per request
# (https://gunicorn.org/reference/settings/#accesslog), and the error log goes
# to stderr, its default (https://gunicorn.org/reference/settings/#errorlog).
exec .venv/bin/gunicorn blog.wsgi \
  --bind "127.0.0.1:$PORT" \
  --workers "$((2 * WORKERS + 1))" \
  --worker-class sync \
  --no-control-socket
