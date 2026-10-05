#!/usr/bin/env bash
# Installs the app and builds it for production, as AdonisJS's deployment
# guide says (docs.adonisjs.com/deployment): `node ace build` compiles the
# TypeScript into build/, which is the app's root in production, and build/
# gets the production dependencies alone, from the lock file the build
# copied into it. Safe to run again: each step starts afresh.
set -euo pipefail
cd "$(dirname "$0")"

# The build needs the development dependencies, TypeScript and the
# assembler, whatever NODE_ENV the shell has, as the guide says for a build
# in a fresh environment.
export NODE_ENV=development

npm ci --no-audit --no-fund
node ace build

cd build
npm ci --omit=dev --no-audit --no-fund
