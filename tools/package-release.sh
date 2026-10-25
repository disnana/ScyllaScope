#!/usr/bin/env bash
# Prepare allowlisted prebuilt assets only. Never installs, tags or publishes.
set -euo pipefail
export LC_ALL=C TZ=UTC GOTOOLCHAIN=local GOWORK=off GOFLAGS=''
unset GOOS GOARCH CGO_ENABLED
cd "$(dirname "$(readlink -f "$0")")/.."
source_root=$PWD
tag=''
output=''
usage() { echo 'Usage: tools/package-release.sh --tag vVERSION --output NEW_DIRECTORY' >&2; }
while (($#)); do
  case "$1" in
    --tag) (($# >= 2)) || { usage; exit 2; }; tag=$2; shift 2 ;;
    --output) (($# >= 2)) || { usage; exit 2; }; output=$2; shift 2 ;;
    *) usage; exit 2 ;;
  esac
done
[[ -n "$tag" && -n "$output" ]] || { usage; exit 2; }
[[ "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z]+([.-][0-9A-Za-z]+)*)?$ && ${#tag} -le 81 ]] || { echo 'Invalid version tag.' >&2; exit 2; }
for command_name in go node python3 git sha256sum tar gzip install mktemp realpath xargs; do
  command -v "$command_name" >/dev/null || { echo "Required build command missing: $command_name" >&2; exit 1; }
done
[[ "$(uname -s)" == Linux ]] || { echo 'Packaging requires a Linux build host.' >&2; exit 1; }
[[ -z "$(git status --porcelain --untracked-files=no)" ]] || { echo 'Commit/review tracked source changes before packaging.' >&2; exit 1; }
source_commit=$(git rev-parse HEAD)
output=$(realpath -m -- "$output")
[[ ! -e "$output" && ! -L "$output" ]] || { echo 'Output must be a new directory; refusing stale assets or overwrite.' >&2; exit 1; }
output_parent=$(dirname -- "$output")
mkdir -p -- "$output_parent"
work=$(mktemp -d "$output_parent/.scyllascope-package.XXXXXX")
source_work=''
cleanup() {
  rm -rf -- "$work"
  if [[ -n "$source_work" ]]; then rm -rf -- "$source_work"; fi
}
trap cleanup EXIT
# Go embeds all static files. Export only the reviewed commit so ignored or
# untracked local configs/logs cannot enter the executable through go:embed.
# Keep this source outside the original repo, even if --output is inside it.
source_work=$(mktemp -d /tmp/scyllascope-source.XXXXXX)
if git -C "$source_work" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  echo 'Source staging unexpectedly belongs to another Git worktree; stopped.' >&2
  exit 1
fi
git -C "$source_root" archive --format=tar "$source_commit" | tar -xf - -C "$source_work"
git -C "$source_root" ls-tree -rz --name-only "$source_commit" > "$work/source-files"
cd "$source_work"
xargs -0 sha256sum -- < "$work/source-files" > "$work/source-manifest.sha256"
verify_source_unchanged() {
  sha256sum --check --status "$work/source-manifest.sha256" || { echo 'Validation changed committed source/dependencies; review and commit before retrying.' >&2; exit 1; }
  [[ "$(git -C "$source_root" rev-parse HEAD)" == "$source_commit" ]] || { echo 'Original source HEAD changed during packaging; stopped.' >&2; exit 1; }
  [[ -z "$(git -C "$source_root" status --porcelain --untracked-files=no)" ]] || { echo 'Original tracked worktree changed during packaging; stopped.' >&2; exit 1; }
}
# A supplied tag cannot relabel the constant compiled into the source.
model_version=$(sed -nE 's/^const Version = "([^"]+)"$/\1/p' internal/model/model.go)
[[ -n "$model_version" && "$tag" == "v$model_version" ]] || { echo 'Tag must exactly equal v + internal/model.Version; change no source during packaging.' >&2; exit 1; }
for required_file in install.sh LICENSE THIRD_PARTY.md LICENSES/RC-MIT.txt tools/collect-notices.py; do
  [[ -f "$required_file" && ! -L "$required_file" ]] || { echo "Missing regular committed release input: $required_file" >&2; exit 1; }
done
assets="$work/assets"
mkdir -p "$assets"

# Check the bundled frontend before and after a real deterministic rebuild.
(cd internal/api/static && sha256sum --check --status ui-manifest.sha256)
cp internal/api/static/ui-manifest.sha256 "$work/frontend-before.sha256"
node web/build.mjs
cmp "$work/frontend-before.sha256" internal/api/static/ui-manifest.sha256
(cd internal/api/static && sha256sum --check --status ui-manifest.sha256)
node --test web/test/*.test.mjs
# build.sh must run successfully now; an old marker cannot skip validation.
./build.sh
sha256sum --check --status BUILD-VERIFIED.sha256
[[ "$(./scyllascope version)" == "ScyllaScope $model_version" ]] || { echo 'Verified host binary version mismatch.' >&2; exit 1; }
verify_source_unchanged
# Preserve exact SDK, production-module and bundled frontend notices in the
# existing archive member, once for both architectures. No private env dump.
python3 tools/collect-notices.py --output "$work/THIRD_PARTY.md"

printf '%s\n' "$model_version" > "$assets/VERSION"
install -m 0755 install.sh "$assets/install.sh"
for target_arch in amd64 arm64; do
  stage="$work/$target_arch"
  mkdir -p "$stage/LICENSES"
  # No -X overrides: model.Version is the source constant above.
  GOOS=linux GOARCH="$target_arch" CGO_ENABLED=0 go build -mod=readonly -trimpath -ldflags='-s -w' -o "$stage/scyllascope" ./cmd/scyllascope
  chmod 0755 "$stage/scyllascope"
  build_info=$(go version -m "$stage/scyllascope")
  [[ "$build_info" == *'CGO_ENABLED=0'* && "$build_info" == *'GOOS=linux'* && "$build_info" == *"GOARCH=$target_arch"* ]] || { echo 'Cross-build metadata mismatch.' >&2; exit 1; }
  install -m 0644 "$assets/VERSION" "$stage/VERSION"
  install -m 0644 LICENSE "$stage/LICENSE"
  install -m 0644 "$work/THIRD_PARTY.md" "$stage/THIRD_PARTY.md"
  install -m 0644 LICENSES/RC-MIT.txt "$stage/LICENSES/RC-MIT.txt"
  archive="$assets/scyllascope-$tag-linux-$target_arch.tar.gz"
  # Explicit five-file list: never archive source trees, configs, logs or state.
  tar --sort=name --mtime=@0 --owner=0 --group=0 --numeric-owner --format=ustar --use-compress-program='gzip -n' -cf "$archive" -C "$stage" \
    scyllascope VERSION LICENSE THIRD_PARTY.md LICENSES/RC-MIT.txt
  printf '%s\n' LICENSE LICENSES/RC-MIT.txt THIRD_PARTY.md VERSION scyllascope > "$work/expected-members"
  tar -tzf "$archive" | sort > "$work/actual-members"
  cmp "$work/expected-members" "$work/actual-members"
done
(
  cd "$assets"
  sha256sum "scyllascope-$tag-linux-amd64.tar.gz" "scyllascope-$tag-linux-arm64.tar.gz" VERSION install.sh > SHA256SUMS
  sha256sum --check --status SHA256SUMS
)
verify_source_unchanged
# Output may have appeared while a local build ran. Never nest into/overwrite it.
mv --no-clobber --no-target-directory -- "$assets" "$output"
[[ ! -e "$assets" ]] || { echo 'Output appeared during build; assets were not published over it.' >&2; exit 1; }
printf 'Prepared %s from committed source %s. No tag, Release, installation or service operation performed.\n' "$tag" "$source_commit"
printf 'Review candidate assets in %s; publication requires separate approval.\n' "$output"
