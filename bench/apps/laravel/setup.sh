#!/usr/bin/env bash
# Installs the app as Laravel's deployment docs say: the packages
# composer.lock pins, without the ones for development, with Composer's
# optimized autoloader, then php artisan optimize, which caches the
# config, the routes, the views and the events. Both can run again.
set -euo pipefail
composer install --no-dev --optimize-autoloader --no-interaction
php artisan optimize
