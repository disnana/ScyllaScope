#!/usr/bin/env bash
# Official prebuilt bootstrap. All effects are gated by the complete footer.
set -euo pipefail

readonly installed_binary='/usr/local/bin/scyllascope'
readonly release_base='https://github.com/disnana/ScyllaScope/releases'
workdir=''

ss_die() { printf 'ScyllaScope: %s\n' "$*" >&2; exit 1; }
ss_need() { command -v "$1" >/dev/null 2>&1 || ss_die "required command not found: $1"; }
ss_usage() {
  cat <<'HELP'
Usage: bash install.sh <install|update|uninstall> [--version vX.Y.Z[-prerelease]]
install/update download and verify an official Linux AMD64/ARM64 release.
Without --version, use the latest stable release. A prerelease must be pinned.
update preserves existing configuration/data and uses automatic failure rollback.
uninstall uses the installed CLI and retains configuration/data/backups by default.
HELP
}
ss_privileged() {
  if [[ $(id -u) == 0 ]]; then "$@"; else
    ss_need sudo
    sudo -- "$@"
  fi
}
ss_fetch() {
  curl --proto '=https' --proto-redir '=https' --tlsv1.2 --fail --location \
    --silent --show-error --connect-timeout 15 --max-time 120 --retry 2 \
    --max-filesize 134217728 --output "$2" "$1" || ss_die "download failed: $1; nothing was installed"
}
ss_tag() {
  local value=${1#v}
  [[ ${#value} -le 80 && $value =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z]+([.-][0-9A-Za-z]+)*)?$ ]] || ss_die 'invalid release version'
  printf 'v%s' "$value"
}
ss_main() {
  # If curl is interrupted before this final argument, no install is started.
  [[ $# -ge 1 && ${!#} == '__SCYLLASCOPE_INSTALLER_COMPLETE_V1__' ]] || ss_die 'incomplete installer; nothing was installed'
  set -- "${@:1:$(( $# - 1 ))}"
  local action=${1:-install} tag='' os arch asset expected listing types version_output
  if (( $# )); then shift; fi
  case "$action" in -h|--help) ss_usage; return;; install|update|uninstall) ;; *) ss_die "unknown action: $action";; esac
  while (( $# )); do
    case "$1" in
      --version) (( $# >= 2 )) || ss_die '--version needs a value'; tag=$(ss_tag "$2"); shift 2;;
      -h|--help) ss_usage; return;;
      *) ss_die "unknown option: $1";;
    esac
  done
  ss_need uname; ss_need id
  os=$(uname -s)
  [[ $os == Linux ]] || ss_die "unsupported OS: $os (Linux with systemd is required)"
  case "$(uname -m)" in x86_64|amd64) arch=amd64;; aarch64|arm64) arch=arm64;; *) ss_die 'unsupported CPU (AMD64/ARM64 only)';; esac
  if [[ -e $installed_binary || -L $installed_binary ]]; then
    [[ -f $installed_binary && ! -L $installed_binary && -x $installed_binary ]] || ss_die 'installed binary is not a regular executable'
  elif [[ $action != install ]]; then
    ss_die 'ScyllaScope is not installed; use install first'
  fi
  if [[ $action == uninstall ]]; then
    [[ -z $tag ]] || ss_die 'uninstall uses the installed version; do not specify --version'
    ss_privileged "$installed_binary" service uninstall
    return
  fi
  for cmd in curl sha256sum tar awk sort mktemp chmod rm; do ss_need "$cmd"; done
  umask 077
  workdir=$(mktemp -d "${TMPDIR:-/tmp}/scyllascope-download.XXXXXXXX")
  trap '[[ -z $workdir ]] || rm -rf -- "$workdir"' EXIT
  trap 'exit 130' INT
  trap 'exit 143' TERM
  trap 'exit 129' HUP
  if [[ -z $tag ]]; then
    ss_fetch "$release_base/latest/download/VERSION" "$workdir/latest-version"
    tag=$(ss_tag "$(cat "$workdir/latest-version")")
  fi
  asset="scyllascope-${tag}-linux-${arch}.tar.gz"
  ss_fetch "$release_base/download/$tag/SHA256SUMS" "$workdir/SHA256SUMS"
  expected=$(awk -v name="$asset" '
    $2 == name { n++; if (NF==2 && length($1)==64 && $1 ~ /^[0-9a-f]+$/) hash=$1 }
    END { if (n!=1 || hash=="") exit 1; print hash }
  ' "$workdir/SHA256SUMS") || ss_die 'release checksum is missing, duplicated or malformed'
  ss_fetch "$release_base/download/$tag/$asset" "$workdir/$asset"
  (cd "$workdir" && printf '%s  %s\n' "$expected" "$asset" | sha256sum --check --status) || ss_die 'checksum mismatch; nothing was installed'
  listing=$(tar -tzf "$workdir/$asset") || ss_die 'invalid release archive'
  [[ $(printf '%s\n' "$listing" | LC_ALL=C sort) == $(printf '%s\n' scyllascope VERSION LICENSE THIRD_PARTY.md LICENSES/RC-MIT.txt | LC_ALL=C sort) ]] || ss_die 'unexpected or duplicate release archive members'
  types=$(tar -tvzf "$workdir/$asset") || ss_die 'cannot inspect release archive'
  printf '%s\n' "$types" | awk 'substr($1,1,1)!="-" { bad=1 } END { exit bad }' || ss_die 'release archive contains a link or non-regular member'
  [[ $(tar -xOzf "$workdir/$asset" VERSION) == "${tag#v}" ]] || ss_die 'archive version does not match release tag'
  # Stream only the verified named member to a new regular private file. Never
  # extract archive paths into system directories or execute a link from a tar.
  tar -xOzf "$workdir/$asset" scyllascope > "$workdir/scyllascope" || ss_die 'cannot unpack release binary'
  [[ -s $workdir/scyllascope ]] || ss_die 'empty release binary'
  chmod 0755 "$workdir/scyllascope"
  version_output=$("$workdir/scyllascope" version) || ss_die 'release binary cannot run on this host'
  [[ $version_output == "ScyllaScope ${tag#v}" ]] || ss_die 'binary version does not match release tag'
  printf 'Verified %s (%s). Running ScyllaScope %s.\n' "$tag" "$arch" "$action"
  # The existing CLI owns configuration retention, database backup and rollback.
  ss_privileged "$workdir/scyllascope" service install
}

ss_main "$@" '__SCYLLASCOPE_INSTALLER_COMPLETE_V1__'
