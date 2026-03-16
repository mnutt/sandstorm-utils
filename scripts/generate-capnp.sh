#!/bin/sh

set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
bin_dir="$repo_root/.cache/bin"
mkdir -p "$bin_dir"
export GOCACHE="${GOCACHE:-/tmp/go-build}"

capnpc_go="$bin_dir/capnpc-go"
go build -o "$capnpc_go" capnproto.org/go/capnp/v3/capnpc-go

capnp_module_dir=$(go list -f '{{.Dir}}' capnproto.org/go/capnp/v3)
std_dir="$capnp_module_dir/std"
schema_dir="$repo_root/schemas"
out_dir="$repo_root/internal/generated"

mkdir -p "$out_dir"
PATH="$bin_dir:$PATH" capnp compile -I"$schema_dir" -I"$std_dir" -ogo:"$out_dir" \
  "$schema_dir/sandstorm/activity.capnp" \
  "$schema_dir/sandstorm/api-session.capnp" \
  "$schema_dir/sandstorm/collection.capnp" \
  "$schema_dir/sandstorm/email.capnp" \
  "$schema_dir/sandstorm/external.capnp" \
  "$schema_dir/sandstorm/grain.capnp" \
  "$schema_dir/sandstorm/hack-session.capnp" \
  "$schema_dir/sandstorm/identity.capnp" \
  "$schema_dir/sandstorm/ip.capnp" \
  "$schema_dir/sandstorm/mime.capnp" \
  "$schema_dir/sandstorm/powerbox.capnp" \
  "$schema_dir/sandstorm/sandstorm-http-bridge.capnp" \
  "$schema_dir/sandstorm/settings.capnp" \
  "$schema_dir/sandstorm/util.capnp" \
  "$schema_dir/sandstorm/web-session.capnp"
