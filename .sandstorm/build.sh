#!/bin/bash

set -euo pipefail

export HOME="${HOME:-/home/vagrant}"

APP_DIR="${APP_DIR:-/opt/app/testapp}"
SOURCE_ROOT="${SOURCE_ROOT:-/opt/app}"
BIN_DIR="${APP_DIR}/bin"

export CGO_ENABLED=0
export GOCACHE="${GOCACHE:-/tmp/go-build}"
export GOPROXY="${GOPROXY:-https://proxy.golang.org,direct}"
export GOSUMDB="${GOSUMDB:-sum.golang.org}"
GO="${GO:-go}"

if [ ! -f "${SOURCE_ROOT}/go.mod" ]; then
  echo "expected go.mod at ${SOURCE_ROOT}/go.mod; mounted work directory is wrong" >&2
  exit 1
fi

if [ ! -d "${APP_DIR}" ]; then
  echo "expected app directory at ${APP_DIR}; current /opt/app contents:" >&2
  ls -la /opt/app >&2 || true
  exit 1
fi

cd "${SOURCE_ROOT}"

rm -rf "${BIN_DIR}"
mkdir -p "${BIN_DIR}"

commands=(
  get-public-id
  get-user-address
  close-session
  open-view
  post-activity
  get-session-request
  get-session-offer
  send-email
  stay-awake
  app-harness
)

for command in "${commands[@]}"; do
  "${GO}" build -mod=mod -o "${BIN_DIR}/${command}" "./cmd/${command}"
done

"${GO}" build -mod=mod -o "${BIN_DIR}/testapp-server" ./testapp/cmd/testapp-server
