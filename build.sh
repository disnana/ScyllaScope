#!/usr/bin/env bash
# No service operations or Go installation/update; fail closed on any build error.
set -euo pipefail
cd "$(dirname "$(readlink -f "$0")")"
# An earlier successful build must not survive as "verified" after ANY failure,
# including missing/old toolchains. Preserve the previous binary, not its marker.
rm -f BUILD-VERIFIED.sha256
stage=''
cleanup() { code=$?; [[ -z "$stage" ]] || rm -f "$stage"; if ((code)); then rm -f BUILD-VERIFIED.sha256; echo 'Build failed. Do not install any pre-existing binary from this directory.' >&2; fi; }
trap cleanup EXIT
export GOTOOLCHAIN=local
if [[ "${SCYLLASCOPE_RACE_TESTS:-1}" != '1' ]]; then
 echo 'Race checks cannot be skipped by build.sh; no verified build will be produced.' >&2
 exit 1
fi
command -v go >/dev/null || { echo 'No Go toolchain found. Nothing was installed or changed.' >&2; exit 1; }
version="$(go env GOVERSION)"
if [[ ! "$version" =~ ^go([0-9]+)\.([0-9]+) ]] || (( BASH_REMATCH[1] < 1 || (BASH_REMATCH[1] == 1 && BASH_REMATCH[2] < 25) )); then
  echo "Pinned original dependencies require Go >=1.25.0; found $version. Nothing was installed or changed." >&2
  exit 1
fi
command -v sha256sum >/dev/null || { echo 'sha256sum is required.' >&2; exit 1; }
echo "==> $(go version)"
(cd internal/api/static && sha256sum --check --status ui-manifest.sha256)
echo '==> Resolve the pinned real dependencies and generate/review go.sum'
go mod tidy
go mod verify
echo '==> Full tests: real production dependency graph'
go test -count=1 ./...
go vet ./...
go test -race -count=1 ./...
stage="$(mktemp "$PWD/.scyllascope-build.XXXXXX")"
go build -trimpath -ldflags='-s -w' -o "$stage" ./cmd/scyllascope
chmod 755 "$stage"
"$stage" version
mv -f "$stage" scyllascope
stage=''
sha256sum scyllascope > BUILD-VERIFIED.sha256
echo 'Build completed; running services and installed Go were not modified.'
echo 'This alone is NOT production approval. Complete the live-cluster/rollback gates in UPGRADE.md.'
