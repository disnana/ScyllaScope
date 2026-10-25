#!/usr/bin/env bash
# Developer-only entry point for a locally verified source build.
set -euo pipefail
cd "$(dirname "$(readlink -f "$0")")/.."
sha256sum --check BUILD-VERIFIED.sha256
./scyllascope version
if (( EUID == 0 )); then
  exec ./scyllascope service install "$@"
else
  exec sudo -- ./scyllascope service install "$@"
fi
