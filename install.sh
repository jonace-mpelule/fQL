#!/bin/sh
# Install a published FQL CLI. No Go, Node.js, Python, or elevated privileges required.
set -eu

fail() { printf 'fql install: %s\n' "$*" >&2; exit 1; }
need() { command -v "$1" >/dev/null 2>&1 || fail "required command not found: $1"; }
version=${FQL_VERSION:-}
install_dir=${FQL_INSTALL_DIR:-"$HOME/.local/bin"}
modify_path=${FQL_MODIFY_PATH:-1}
while [ "$#" -gt 0 ]; do
  case "$1" in
    --version) [ "$#" -ge 2 ] || fail '--version needs a value'; version=$2; shift 2 ;;
    --install-dir) [ "$#" -ge 2 ] || fail '--install-dir needs a value'; install_dir=$2; shift 2 ;;
    --no-modify-path) modify_path=0; shift ;;
    --help|-h) printf '%s\n' 'Usage: install.sh [--version VERSION] [--install-dir ABSOLUTE_PATH] [--no-modify-path]'; exit 0 ;;
    *) fail "unknown argument: $1" ;;
  esac
done
case "$install_dir" in /*) ;; *) fail 'installation directory must be an absolute path' ;; esac
case "$install_dir" in *'
'*) fail 'installation directory cannot contain a newline' ;; esac
for tool in curl tar awk grep sed uname mktemp chmod cp mv mkdir rm head dirname; do need "$tool"; done
case "$(uname -s)" in Darwin) os=darwin ;; Linux) os=linux ;; *) fail 'supported systems: macOS and Linux' ;; esac
case "$(uname -m)" in arm64|aarch64) arch=arm64 ;; x86_64|amd64) arch=amd64 ;; *) fail 'supported CPU architectures: arm64 and amd64' ;; esac
if command -v sha256sum >/dev/null 2>&1; then hash_tool=sha256sum
elif command -v shasum >/dev/null 2>&1; then hash_tool=shasum
elif command -v openssl >/dev/null 2>&1; then hash_tool=openssl
else fail 'install sha256sum, shasum, or openssl for checksum verification'; fi

tmp=$(mktemp -d "${TMPDIR:-/tmp}/fql-install.XXXXXX")
stage=''
cleanup() { rm -rf "$tmp"; if [ -n "$stage" ]; then rm -f "$stage"; fi; }
trap cleanup 0
trap 'exit 130' INT
trap 'exit 143' TERM HUP
fetch() { curl --fail --silent --show-error --location --proto '=https' --tlsv1.2 --retry 3 --connect-timeout 15 --max-time 180 --output "$2" "$1"; }
repo=jonace-mpelule/fQL
if [ -z "$version" ]; then
  if ! fetch "https://api.github.com/repos/$repo/releases/latest" "$tmp/release.json" 2>/dev/null; then
    fetch "https://api.github.com/repos/$repo/releases?per_page=1" "$tmp/release.json" || fail 'cannot find a published release; try --version VERSION'
  fi
  version=$(sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"v\([^"]*\)".*/\1/p' "$tmp/release.json" | head -n 1)
fi
version=${version#v}
printf '%s\n' "$version" | grep -Eq '^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z][0-9A-Za-z.-]*)?$' || fail 'no valid release version found; use --version VERSION'
asset="fql_${version}_${os}_${arch}.tar.gz"
base="https://github.com/$repo/releases/download/v$version"
printf 'Installing FQL %s for %s/%s…\n' "$version" "$os" "$arch"
fetch "$base/SHA256SUMS" "$tmp/SHA256SUMS" || fail 'cannot download checksums'
fetch "$base/$asset" "$tmp/$asset" || fail 'cannot download the platform archive'
expected=$(awk -v name="$asset" '$2 == name { count++; hash=$1 } END { if (count==1) print hash; else exit 1 }' "$tmp/SHA256SUMS") || fail 'checksum manifest must contain exactly one entry for the archive'
printf '%s\n' "$expected" | grep -Eq '^[0-9a-f]{64}$' || fail 'invalid checksum entry'
case "$hash_tool" in
  sha256sum) actual=$(sha256sum "$tmp/$asset" | awk '{print $1}') ;;
  shasum) actual=$(shasum -a 256 "$tmp/$asset" | awk '{print $1}') ;;
  openssl) actual=$(openssl dgst -sha256 "$tmp/$asset" | awk '{print $NF}') ;;
esac
[ "$actual" = "$expected" ] || fail 'checksum mismatch; existing installation was not changed'
[ "$(tar -tzf "$tmp/$asset")" = 'fql' ] || fail 'unexpected archive contents'
case "$(tar -tvzf "$tmp/$asset")" in -*) ;; *) fail 'archive must contain a regular executable' ;; esac
tar -xzf "$tmp/$asset" -C "$tmp" fql
[ -f "$tmp/fql" ] && [ ! -L "$tmp/fql" ] || fail 'archive executable is not a regular file'
chmod 755 "$tmp/fql"
[ "$("$tmp/fql" version)" = "fql $version" ] || fail 'downloaded executable failed its version check'
mkdir -p "$install_dir"
[ ! -d "$install_dir/fql" ] || fail 'destination fql is a directory'
stage=$(mktemp "$install_dir/.fql.XXXXXX")
cp "$tmp/fql" "$stage"
chmod 755 "$stage"
mv -f "$stage" "$install_dir/fql"
stage=''
printf 'Installed %s/fql\n' "$install_dir"
case ":$PATH:" in *":$install_dir:"*) printf '%s\n' 'Run: fql version'; exit 0 ;; esac
# Escape double-quoted shell content; never evaluate an installation path.
escaped=$(printf '%s' "$install_dir" | sed 's/\\/\\\\/g; s/"/\\"/g; s/\$/\\$/g; s/`/\\`/g')
path_line="export PATH=\"$escaped:\$PATH\""
if [ "$modify_path" = 1 ]; then
  case "${SHELL:-/bin/sh}" in
    */zsh) profile="${ZDOTDIR:-$HOME}/.zshrc" ;;
    */bash) profile="$HOME/.bashrc" ;;
    */sh|*/dash|*/ksh) profile="$HOME/.profile" ;;
    *) profile='' ;;
  esac
  if [ -n "$profile" ]; then
    mkdir -p "$(dirname "$profile")"
    if ! grep -Fqx "$path_line" "$profile" 2>/dev/null; then
      if printf '\n# FQL CLI\n%s\n' "$path_line" >> "$profile"; then
        printf 'Added FQL to PATH in %s.\n' "$profile"
      else
        printf 'Could not update %s; add the PATH line below manually.\n' "$profile" >&2
      fi
    fi
  fi
fi
printf 'For this terminal, run:\n  %s\n  fql version\n' "$path_line"
