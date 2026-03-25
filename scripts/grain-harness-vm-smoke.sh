#!/bin/bash
set -euo pipefail

pkill -f "/opt/app/testapp/bin/app-harness serve --root /tmp/gh-serve-test" || true
rm -rf /tmp/gh-serve-test

cd /opt/app
bash .sandstorm/build.sh

nohup /opt/app/testapp/bin/app-harness serve \
  --root /tmp/gh-serve-test \
  --pkg-def /opt/app/.sandstorm/sandstorm-pkgdef.capnp:pkgdef \
  --port 3011 \
  >/tmp/gh-serve-test.log 2>&1 </dev/null &
