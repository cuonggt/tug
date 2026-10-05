#!/usr/bin/env bash
# Prints what the app runs on, a line each, "Name version", from the Python
# in .venv and the packages installed there.
set -euo pipefail
cd "$(dirname "$0")"

.venv/bin/python - <<'EOF'
import platform
from importlib.metadata import version

print('Python', platform.python_version())
for name in ('Django', 'inertia-django', 'gunicorn'):
    print(name, version(name))
EOF
