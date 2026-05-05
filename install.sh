#!/usr/bin/env bash
# sina installer
#
#   curl -fsSL https://get.ottomind.ai/sina | bash
#   curl -fsSL https://get.ottomind.ai/sina | sudo bash
#
# Env overrides:
#   SINA_VERSION=0.2.0   # pin a version (default: latest)
#   SINA_REPO=owner/repo
#   SINA_PREFIX=/usr/local/bin
#   SINA_DEBUG=1         # verbose

set -euo pipefail

REPO="${SINA_REPO:-19011022/sina}"
PREFIX="${SINA_PREFIX:-/usr/local/bin}"
BIN="sina"

# ---------- pretty output ----------
if [[ -t 1 ]]; then
  c_ok=$'\033[1;32m'; c_warn=$'\033[1;33m'; c_err=$'\033[1;31m'
  c_dim=$'\033[2m';   c_acc=$'\033[1;36m'; c_off=$'\033[0m'
else
  c_ok=""; c_warn=""; c_err=""; c_dim=""; c_acc=""; c_off=""
fi
ok()    { printf "%s✓%s %s\n" "$c_ok"   "$c_off" "$*"; }
info()  { printf "%s·%s %s\n" "$c_dim"  "$c_off" "$*"; }
warn()  { printf "%s!%s %s\n" "$c_warn" "$c_off" "$*"; }
fail()  { printf "%s✗%s %s\n" "$c_err"  "$c_off" "$*" >&2; exit 1; }
debug() { [[ -n "${SINA_DEBUG:-}" ]] && printf "%s[debug]%s %s\n" "$c_dim" "$c_off" "$*" || true; }

trap 'fail "install aborted (line $LINENO)"' ERR

# ---------- platform detection ----------
detect_os() {
  case "$(uname -s)" in
    Linux*)   echo linux  ;;
    Darwin*)  echo darwin ;;
    *) fail "unsupported OS: $(uname -s)" ;;
  esac
}

detect_arch() {
  case "$(uname -m)" in
    x86_64|amd64)  echo amd64 ;;
    aarch64|arm64) echo arm64 ;;
    *) fail "unsupported arch: $(uname -m)" ;;
  esac
}

OS="$(detect_os)"
ARCH="$(detect_arch)"
ok "platform: ${c_acc}${OS}/${ARCH}${c_off}"

# ---------- prerequisites ----------
need_cmd() { command -v "$1" >/dev/null 2>&1 || fail "missing required command: $1"; }
need_cmd uname
need_cmd tar
need_cmd install
HTTP=""
if command -v curl >/dev/null 2>&1; then
  HTTP="curl -fsSL"
elif command -v wget >/dev/null 2>&1; then
  HTTP="wget -qO-"
else
  fail "need curl or wget"
fi
debug "http client: $HTTP"

# ---------- privilege ----------
SUDO=""
if [[ "$(id -u)" -ne 0 ]]; then
  if [[ -w "$PREFIX" ]]; then
    SUDO=""
  elif command -v sudo >/dev/null 2>&1; then
    SUDO="sudo"
  else
    fail "$PREFIX is not writable and sudo is not installed"
  fi
fi

# ---------- resolve version ----------
VERSION="${SINA_VERSION:-}"
if [[ -z "$VERSION" ]]; then
  info "resolving latest version from github.com/$REPO ..."
  api="https://api.github.com/repos/$REPO/releases/latest"
  body="$($HTTP "$api" 2>/dev/null || true)"
  if [[ -z "$body" ]]; then
    fail "could not query $api — set SINA_VERSION to install a specific version"
  fi
  # Extract "tag_name": "v0.2.0" → 0.2.0
  VERSION="$(echo "$body" \
    | grep -E '"tag_name"[[:space:]]*:' \
    | head -n1 \
    | sed -E 's/.*"tag_name"[[:space:]]*:[[:space:]]*"v?([^"]+)".*/\1/')"
  if [[ -z "$VERSION" ]]; then
    fail "could not parse latest version from GitHub API response"
  fi
fi
ok "version: ${c_acc}${VERSION}${c_off}"

# ---------- download ----------
ASSET="sina_${VERSION}_${OS}_${ARCH}.tar.gz"
URL="https://github.com/$REPO/releases/download/v${VERSION}/${ASSET}"
SUMS_URL="https://github.com/$REPO/releases/download/v${VERSION}/SHA256SUMS"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

info "downloading $ASSET ..."
debug "url: $URL"
$HTTP "$URL" > "$tmp/$ASSET" || fail "download failed: $URL"

# ---------- verify checksum (best effort) ----------
if command -v sha256sum >/dev/null 2>&1; then
  SHASUM_BIN="sha256sum"
elif command -v shasum >/dev/null 2>&1; then
  SHASUM_BIN="shasum -a 256"
else
  SHASUM_BIN=""
fi

if [[ -n "$SHASUM_BIN" ]]; then
  info "verifying checksum ..."
  expected="$($HTTP "$SUMS_URL" 2>/dev/null | awk -v f="$ASSET" '$2 == f {print $1}')"
  if [[ -n "$expected" ]]; then
    actual="$($SHASUM_BIN "$tmp/$ASSET" | awk '{print $1}')"
    if [[ "$expected" != "$actual" ]]; then
      fail "checksum mismatch: expected $expected got $actual"
    fi
    ok "checksum verified"
  else
    warn "no checksum entry found in SHA256SUMS — skipping verification"
  fi
else
  warn "neither sha256sum nor shasum present — skipping verification"
fi

# ---------- extract & install ----------
tar -xzf "$tmp/$ASSET" -C "$tmp"
[[ -f "$tmp/$BIN" ]] || fail "archive did not contain expected binary: $BIN"

info "installing to $PREFIX/$BIN ..."
$SUDO install -m 0755 "$tmp/$BIN" "$PREFIX/$BIN"

# ---------- smoke test ----------
if "$PREFIX/$BIN" --version >/dev/null 2>&1; then
  ok "$($PREFIX/$BIN --version)"
else
  warn "binary installed but --version failed"
fi

echo
printf "  Run:  %ssina%s             %s# interactive TUI%s\n"        "$c_acc" "$c_off" "$c_dim" "$c_off"
printf "        %ssina --plain%s     %s# scriptable / cron%s\n"      "$c_acc" "$c_off" "$c_dim" "$c_off"
echo
