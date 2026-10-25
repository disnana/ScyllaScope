#!/usr/bin/env bash
# Install only the locally verified build. Does not download or alter ScyllaDB.
set -euo pipefail
cd "$(dirname "$(readlink -f "$0")")"
sha256sum --check BUILD-VERIFIED.sha256
./scyllascope version
if (( EUID == 0 )); then
 exec ./scyllascope service install "$@"
else
 exec sudo ./scyllascope service install "$@"
fi
