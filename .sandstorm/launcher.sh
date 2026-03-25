#!/bin/bash
set -euo pipefail

export PATH="/opt/app/testapp/bin:/usr/local/bin:/usr/bin:/bin"
export PORT="${PORT:-3000}"
export HOME="${HOME:-/var}"
export SANDSTORM=1
export TMPDIR="${TMPDIR:-/tmp}"

exec /sandstorm-http-bridge "${PORT}" -- /opt/app/testapp/bin/testapp-server
