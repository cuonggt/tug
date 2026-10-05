#!/usr/bin/env bash
# Makes the app's virtualenv, .venv, if it isn't there, and installs
# requirements.txt into it. Run again, it changes nothing: pip leaves alone
# what's installed at the version pinned.
set -euo pipefail
cd "$(dirname "$0")"

if [ ! -x .venv/bin/python ]; then
  python3 -m venv .venv
fi
.venv/bin/python -m pip install --quiet --disable-pip-version-check --requirement requirements.txt
