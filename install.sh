#!/bin/sh
set -e

REPO="chiperka/cli"
INSTALL_DIR="/usr/local/bin"
BINARY="chiperka"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

info() { printf "${GREEN}▸${NC} %s\n" "$1"; }
warn() { printf "${YELLOW}▸${NC} %s\n" "$1"; }
error() { printf "${RED}✗${NC} %s\n" "$1" >&2; exit 1; }

# Detect OS
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$OS" in
    darwin) OS="darwin" ;;
    linux)  OS="linux" ;;
    *)      error "Unsupported OS: $OS" ;;
esac

# Detect architecture
ARCH=$(uname -m)
case "$ARCH" in
    x86_64|amd64)  ARCH="amd64" ;;
    arm64|aarch64)  ARCH="arm64" ;;
    *)              error "Unsupported architecture: $ARCH" ;;
esac

# Get latest version
info "Detecting latest version..."
VERSION=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')
if [ -z "$VERSION" ]; then
    error "Could not determine latest version"
fi
info "Latest version: ${VERSION}"

# Download
FILENAME="${BINARY}_${VERSION}_${OS}_${ARCH}.tar.gz"
URL="https://github.com/${REPO}/releases/download/${VERSION}/${FILENAME}"

info "Downloading ${FILENAME}..."
TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT

curl -fsSL "$URL" -o "${TMP_DIR}/${FILENAME}" || error "Download failed. Check https://github.com/${REPO}/releases"

# Extract
info "Extracting..."
tar -xzf "${TMP_DIR}/${FILENAME}" -C "$TMP_DIR"

# Install
info "Installing to ${INSTALL_DIR}..."
if [ -w "$INSTALL_DIR" ]; then
    mv "${TMP_DIR}/${BINARY}" "${INSTALL_DIR}/${BINARY}"
else
    warn "Need sudo to install to ${INSTALL_DIR}"
    sudo mv "${TMP_DIR}/${BINARY}" "${INSTALL_DIR}/${BINARY}"
fi
chmod +x "${INSTALL_DIR}/${BINARY}"

# Verify
INSTALLED_VERSION=$("${INSTALL_DIR}/${BINARY}" --version 2>/dev/null || echo "unknown")
info "Installed chiperka ${INSTALLED_VERSION}"
printf "\n${GREEN}✓${NC} Chiperka installed successfully!\n"
printf "  Run ${YELLOW}chiperka --help${NC} to get started.\n\n"