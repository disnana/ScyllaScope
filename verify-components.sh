#!/usr/bin/env bash
# Selected real source packages, without external adapters or substitute modules.
set -euo pipefail
cd "$(dirname "$(readlink -f "$0")")"
export GOTOOLCHAIN=local GOPROXY=off
command -v go >/dev/null || { echo 'An existing Go toolchain is required; nothing was installed.' >&2; exit 1; }
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
# Use an explicitly scoped alternate module, NOT the production dependency graph.
# The same source files are tested; there are NO replacement/fake dependency modules.
printf 'module scyllascope\n\ngo 1.23.0\n' > "$tmp/components.mod"
pkgs=(./internal/model ./internal/core ./internal/monitor ./internal/alerts ./internal/api ./internal/security ./internal/probe ./internal/process ./internal/settings ./internal/service ./internal/repository ./internal/testkit ./cmd/devserver)
echo 'SCOPE: dependency-free components only; excludes config, SQLite, CQL, doctor and production CLI.' >&2
echo 'This does NOT approve a production build or replace ./build.sh.' >&2
go test -modfile="$tmp/components.mod" "$@" "${pkgs[@]}"
