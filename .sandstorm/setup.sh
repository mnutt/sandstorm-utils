#!/bin/bash

set -euo pipefail

GO_VERSION="1.24.4"
GO_ARCHIVE="go${GO_VERSION}.linux-amd64.tar.gz"
GO_URL="https://go.dev/dl/${GO_ARCHIVE}"
CACHE_DIR="/host-dot-sandstorm/caches"
GO_ROOT="/usr/local/go"

mkdir -p "${CACHE_DIR}"

need_install=1
if [ -x "${GO_ROOT}/bin/go" ]; then
  current_version="$("${GO_ROOT}/bin/go" version | awk '{print $3}')"
  if [ "${current_version}" = "go${GO_VERSION}" ]; then
    need_install=0
  fi
fi

if [ "${need_install}" -eq 1 ]; then
  archive_path="${CACHE_DIR}/${GO_ARCHIVE}"
  if [ ! -f "${archive_path}" ]; then
    curl --fail --location --silent --show-error --output "${archive_path}.partial" "${GO_URL}"
    mv "${archive_path}.partial" "${archive_path}"
  fi

  rm -rf "${GO_ROOT}"
  tar -C /usr/local -xzf "${archive_path}"
fi

ln -sf "${GO_ROOT}/bin/go" /usr/local/bin/go
ln -sf "${GO_ROOT}/bin/gofmt" /usr/local/bin/gofmt
