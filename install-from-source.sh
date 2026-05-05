#!/usr/bin/env bash
# Install sina to /usr/local/bin/sina by building from source.
#
# Usage:
#   ./install-from-source.sh           # build & install
#   ./install-from-source.sh --no-go   # don't try to install Go via apt
#   ./install-from-source.sh --prefix=/opt/bin
#
# Run from inside the unpacked sina source directory. Re-runnable.

set -euo pipefail

# ---- locate source directory ----
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# ---- arguments ----
INSTALL_GO=1
INSTALL_DIR="/usr/local/bin"
for arg in "$@"; do
  case "$arg" in
    --no-go)    INSTALL_GO=0 ;;
    --prefix=*) INSTALL_DIR="${arg#--prefix=}" ;;
    -h|--help)
      sed -n '1,12p' "$0"
      exit 0 ;;
    *) echo "unknown flag: $arg" >&2; exit 1 ;;
  esac
done

# ---- helpers ----
c_red()    { printf '\033[1;31m%s\033[0m' "$*"; }
c_green()  { printf '\033[1;32m%s\033[0m' "$*"; }
c_yellow() { printf '\033[1;33m%s\033[0m' "$*"; }
c_dim()    { printf '\033[2m%s\033[0m'   "$*"; }
ok()    { echo "$(c_green '[ok]')   $*"; }
warn()  { echo "$(c_yellow '[warn]') $*"; }
fail()  { echo "$(c_red '[fail]') $*" >&2; exit 1; }
info()  { echo "$(c_dim '[..]')   $*"; }

# ---- need bash ----
if [[ -z "${BASH_VERSION:-}" ]]; then
  fail "this installer needs bash"
fi

# ---- need root for /usr/local/bin ----
SUDO=""
if [[ "$EUID" -ne 0 ]]; then
  if command -v sudo >/dev/null 2>&1; then
    SUDO="sudo"
  else
    fail "need root or sudo to write to $INSTALL_DIR"
  fi
fi

# ---- ensure source files are here ----
required=(go.mod main.go collect.go decide.go render.go styles.go model.go types.go)
for f in "${required[@]}"; do
  [[ -f "$SCRIPT_DIR/$f" ]] || fail "source file missing: $f (run from the sina source directory)"
done
ok "source files found"

# ---- ensure Go is available ----
if ! command -v go >/dev/null 2>&1; then
  if [[ $INSTALL_GO -eq 1 ]] && command -v apt-get >/dev/null 2>&1; then
    info "installing golang-go via apt..."
    $SUDO apt-get update -qq
    $SUDO apt-get install -y -qq golang-go
  else
    fail "Go is not installed. Install Go 1.22+ then re-run."
  fi
fi
GO_BIN="$(command -v go)"
GO_VER="$($GO_BIN version)"
ok "$GO_VER"

# ---- build ----
info "fetching dependencies..."
export GOFLAGS="${GOFLAGS:-}"

# Some networks block proxy.golang.org; if the default proxy fails we retry
# with GOPROXY=direct (which talks straight to GitHub) and disable the sumdb.
build_dir="$(mktemp -d)"
trap 'rm -rf "$build_dir"' EXIT
out_bin="$build_dir/sina"

mod_ok=0
( cd "$SCRIPT_DIR" && "$GO_BIN" mod tidy ) >/dev/null 2>&1 && mod_ok=1
if [[ $mod_ok -eq 0 ]]; then
  warn "default GOPROXY failed — retrying with GOPROXY=direct"
  ( cd "$SCRIPT_DIR" && GOPROXY=direct GOSUMDB=off "$GO_BIN" mod tidy )
fi

info "building..."
( cd "$SCRIPT_DIR" && "$GO_BIN" build -ldflags="-s -w" -o "$out_bin" . )
ok "built $(du -h "$out_bin" | awk '{print $1}') binary"

# ---- install ----
info "installing to $INSTALL_DIR/sina..."
$SUDO install -m 0755 "$out_bin" "$INSTALL_DIR/sina"
ok "$INSTALL_DIR/sina"

# ---- smoke test ----
if "$INSTALL_DIR/sina" --version >/dev/null 2>&1; then
  ok "smoke test: $($INSTALL_DIR/sina --version)"
else
  warn "binary installed but --version failed"
fi

echo
echo "Run:  $(c_green sina)         # interactive TUI"
echo "      $(c_green sina --plain) # piped / scripted"
