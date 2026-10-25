#!/usr/bin/env bash
# State and credentials are retained by default. --purge is explicitly destructive.
set -euo pipefail
if [[ "$(id -u)" == 0 ]]; then
  exec /usr/local/bin/scyllascope service uninstall "$@"
else
  exec sudo /usr/local/bin/scyllascope service uninstall "$@"
fi
