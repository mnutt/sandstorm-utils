#!/bin/bash
set -euo pipefail

pkill -f "/opt/app/testapp/bin/app-harness serve --root /tmp/gh-serve-test" || true
rm -rf /tmp/gh-serve-test
cat >/tmp/gh-serve-mocks.json <<'EOF'
{
  "publicId": {
    "publicId": "mock-public-id",
    "hostname": "mock.local.sandstorm.test",
    "autoUrl": "https://mock.local.sandstorm.test/shared/mock-public-id",
    "isDemoUser": true
  },
  "userAddress": {
    "address": "mock-user@example.com",
    "name": "Mock User"
  }
}
EOF

cd /opt/app
bash .sandstorm/build.sh

nohup /opt/app/testapp/bin/app-harness serve \
  --root /tmp/gh-serve-test \
  --pkg-def /opt/app/.sandstorm/sandstorm-pkgdef.capnp:pkgdef \
  --mocks /tmp/gh-serve-mocks.json \
  --port 3011 \
  >/tmp/gh-serve-test.log 2>&1 </dev/null &
