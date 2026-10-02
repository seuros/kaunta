#!/usr/bin/env bash
set -euo pipefail
IFS=$'\n\t'

REPO="seuros/kaunta"
DEFAULT_SYSTEM_PREFIX="/usr/local/bin"

usage() {
  cat <<'EOF'
Usage: install.sh [options]

Options:
  --prefix <dir>    Install kaunta into the specified directory
  --system          Install into /usr/local/bin
  --version <tag>   Install a specific tag, with or without a leading v
  --yes             Permit sudo without an interactive confirmation
  --no-sudo         Never use sudo
  -h, --help        Show this help text

Environment:
  KAUNTA_INSTALL_PREFIX
  KAUNTA_INSTALL_DIR
  KAUNTA_INSTALL_VERSION
  KAUNTA_INSTALL_ASSUME_SUDO  always, never, or prompt

The default install prefix is ~/.local/bin.
EOF
}

info() { printf '==> %s\n' "$*"; }
warn() { printf 'WARN: %s\n' "$*" >&2; }
fail() { printf 'ERROR: %s\n' "$*" >&2; exit 1; }

expand_path() {
  case "$1" in
    "~")
      [ -n "${HOME:-}" ] || fail "HOME is not set"
      printf '%s\n' "$HOME"
      ;;
    "~/"*)
      [ -n "${HOME:-}" ] || fail "HOME is not set"
      printf '%s/%s\n' "$HOME" "${1#~/}"
      ;;
    *)
      printf '%s\n' "$1"
      ;;
  esac
}

parse_args() {
  REQUESTED_VERSION="${KAUNTA_INSTALL_VERSION:-}"
  PREFIX="${KAUNTA_INSTALL_PREFIX:-${KAUNTA_INSTALL_DIR:-}}"
  SYSTEM_INSTALL=0
  ASSUME_YES=0
  SUDO_MODE="${KAUNTA_INSTALL_ASSUME_SUDO:-prompt}"

  while [ "$#" -gt 0 ]; do
    case "$1" in
      --prefix)
        [ "$#" -ge 2 ] || fail "--prefix requires a directory"
        PREFIX="$2"
        shift
        ;;
      --system)
        SYSTEM_INSTALL=1
        ;;
      --version)
        [ "$#" -ge 2 ] || fail "--version requires a tag"
        REQUESTED_VERSION="$2"
        shift
        ;;
      --yes|--assume-yes)
        ASSUME_YES=1
        ;;
      --no-sudo)
        SUDO_MODE="never"
        ;;
      -h|--help)
        usage
        exit 0
        ;;
      *)
        fail "unknown option: $1"
        ;;
    esac
    shift
  done

  case "$SUDO_MODE" in
    always|never|prompt) ;;
    *) fail "KAUNTA_INSTALL_ASSUME_SUDO must be always, never, or prompt" ;;
  esac

  if [ -z "$PREFIX" ]; then
    if [ "$SYSTEM_INSTALL" -eq 1 ] || [ -z "${HOME:-}" ]; then
      PREFIX="$DEFAULT_SYSTEM_PREFIX"
    else
      PREFIX="$HOME/.local/bin"
    fi
  fi
  PREFIX=$(expand_path "$PREFIX")
  if [ "$ASSUME_YES" -eq 1 ] && [ "$SUDO_MODE" = "prompt" ]; then
    SUDO_MODE="always"
  fi
}

detect_http_client() {
  if command -v curl >/dev/null 2>&1; then
    HTTP_CLIENT="curl"
  elif command -v wget >/dev/null 2>&1; then
    HTTP_CLIENT="wget"
  else
    fail "curl or wget is required"
  fi
}

fetch() {
  if [ "$HTTP_CLIENT" = "curl" ]; then
    curl -fsSL --proto '=https' --tlsv1.2 "$1"
  else
    wget -qO- "$1"
  fi
}

download() {
  if [ "$HTTP_CLIENT" = "curl" ]; then
    curl -fsSL --proto '=https' --tlsv1.2 -o "$2" "$1"
  else
    wget -q -O "$2" "$1"
  fi
}

detect_platform() {
  case "$(uname -s | tr '[:upper:]' '[:lower:]')" in
    linux) OS="linux" ;;
    darwin) OS="darwin" ;;
    freebsd) OS="freebsd" ;;
    *) fail "unsupported operating system: $(uname -s)" ;;
  esac

  case "$(uname -m)" in
    x86_64|amd64) ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *) fail "unsupported architecture: $(uname -m)" ;;
  esac

  if [ "$OS" = "freebsd" ] && [ "$ARCH" != "amd64" ]; then
    fail "FreeBSD releases are currently available only for amd64"
  fi
}

resolve_tag() {
  if [ -n "$REQUESTED_VERSION" ]; then
    TAG="v${REQUESTED_VERSION#v}"
    return
  fi

  local release_json
  release_json=$(fetch "https://api.github.com/repos/${REPO}/releases/latest") \
    || fail "could not query the latest GitHub release"
  TAG=$(printf '%s\n' "$release_json" | awk -F'"' '/"tag_name":/ {print $4; exit}')
  [ -n "$TAG" ] || fail "could not determine the latest release tag"
}

verify_checksum() {
  local archive="$1"
  local checksum="$2"
  if command -v sha256sum >/dev/null 2>&1; then
    (cd "$TMP_DIR" && sha256sum --check "$(basename "$checksum")")
  elif command -v shasum >/dev/null 2>&1; then
    (cd "$TMP_DIR" && shasum -a 256 --check "$(basename "$checksum")")
  else
    warn "sha256sum/shasum unavailable; skipping checksum verification"
  fi
}

download_release() {
  TMP_DIR=$(mktemp -d 2>/dev/null || mktemp -d -t kaunta-install)
  trap 'rm -rf "$TMP_DIR"' EXIT

  local archive_name="kaunta_${OS}_${ARCH}.tar.gz"
  local base_url="https://github.com/${REPO}/releases/download/${TAG}"
  local archive="${TMP_DIR}/${archive_name}"
  local checksum="${archive}.sha256"

  info "Downloading Kaunta ${TAG} for ${OS}/${ARCH}"
  download "${base_url}/${archive_name}" "$archive" \
    || fail "release asset ${archive_name} was not found"
  [ -s "$archive" ] || fail "downloaded release archive is empty"

  if download "${base_url}/${archive_name}.sha256" "$checksum" 2>/dev/null; then
    verify_checksum "$archive" "$checksum" || fail "release checksum verification failed"
  else
    warn "release checksum is unavailable; continuing for compatibility"
  fi

  mkdir "${TMP_DIR}/extract"
  tar -xzf "$archive" -C "${TMP_DIR}/extract"
  DOWNLOADED_BIN=$(find "${TMP_DIR}/extract" -type f -name 'kaunta*' | head -n 1)
  [ -n "$DOWNLOADED_BIN" ] && [ -s "$DOWNLOADED_BIN" ] \
    || fail "Kaunta binary was not found in ${archive_name}"
  chmod 0755 "$DOWNLOADED_BIN"
}

prepare_install() {
  if mkdir -p "$PREFIX" 2>/dev/null && [ -w "$PREFIX" ]; then
    USE_SUDO=0
    return
  fi
  [ "$SUDO_MODE" != "never" ] \
    || fail "install prefix ${PREFIX} is not writable and sudo is disabled"
  command -v sudo >/dev/null 2>&1 \
    || fail "sudo is required to install into ${PREFIX}"

  if [ "$SUDO_MODE" = "prompt" ]; then
    [ -t 0 ] && [ -t 1 ] \
      || fail "cannot prompt for sudo; use --yes or a writable --prefix"
    printf 'Install into %s with sudo? [y/N] ' "$PREFIX" >&2
    read -r reply
    case "$reply" in
      y|Y|yes|YES) ;;
      *) fail "installation cancelled" ;;
    esac
  fi
  USE_SUDO=1
}

install_binary() {
  local destination="${PREFIX}/kaunta"
  if [ "$USE_SUDO" -eq 1 ]; then
    sudo mkdir -p "$PREFIX"
    sudo install -m 0755 "$DOWNLOADED_BIN" "$destination"
  else
    install -m 0755 "$DOWNLOADED_BIN" "$destination"
  fi
  info "Installed Kaunta to ${destination}"
  if ! "$destination" --version; then
    warn "${destination} was installed but could not be executed; check the binary matches this platform"
  fi
  case ":${PATH:-}:" in
    *":${PREFIX}:"*) ;;
    *) warn "${PREFIX} is not on PATH" ;;
  esac
}

parse_args "$@"
detect_http_client
detect_platform
resolve_tag
download_release
prepare_install
install_binary
