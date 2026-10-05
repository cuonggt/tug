#!/usr/bin/env bash
# Serves the app on 127.0.0.1:$PORT with Laravel Octane, Laravel's own
# application server, on Swoole. Octane's docs start a worker for each
# CPU core; the runner passes that count as $WORKERS. The rest is
# Octane's defaults: a worker restarted after 500 requests, as its docs
# say, and a task worker for each CPU core. With APP_ENV production, as
# .env has it, Octane writes no line for each request.
set -euo pipefail
# OPcache on, as a production PHP has it: PHP's CLI leaves it off. This
# flag is for octane:start, which starts the server and watches it; the
# server's own php takes it from config/octane.php, which also keeps
# Octane from clearing it, the one default changed.
exec php -d opcache.enable_cli=1 artisan octane:start --server=swoole \
  --host=127.0.0.1 --port="$PORT" --workers="$WORKERS"
