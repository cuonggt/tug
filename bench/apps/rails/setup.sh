#!/usr/bin/env bash
# Installs the gems Gemfile.lock pins into vendor/bundle, in Bundler's
# deployment mode, as the Dockerfile `rails new` writes does for production:
# the lockfile is used as it is, and an install that would change it fails.
#
# There's no build step: the app has no assets, and `rails new --minimal`
# leaves out bootsnap, so there's no cache to precompile.
set -euo pipefail
cd "$(dirname "$0")"

bundle config set --local deployment true
bundle config set --local path vendor/bundle
bundle install
