#!/bin/sh
set -eu

# GitHub repository
REPO="dsnbyte/telesend"
GITHUB_URL="https://github.com/$REPO"

# Default values
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"
VERSION="${VERSION:-latest}"

print_banner() {
    echo "========================================"
    echo "          Telesend Installer            "
    echo "========================================"
}

print_help() {
    print_banner
    echo "Usage: install.sh [options]"
    echo ""
    echo "Options:"
    echo "  --help          Show this help message"
    echo ""
    echo "Environment variables:"
    echo "  INSTALL_DIR     Directory to install telesend (default: \$HOME/.local/bin)"
    echo "  VERSION         Version to install (default: latest)"
    echo ""
    echo "Example:"
    echo "  curl -fsSL https://raw.githubusercontent.com/$REPO/main/install.sh | sh"
    echo "  curl -fsSL https://raw.githubusercontent.com/$REPO/main/install.sh | VERSION=v0.2.0 sh"
    exit 0
}

if [ "$#" -gt 0 ] && [ "$1" = "--help" ]; then
    print_help
fi

print_banner

# OS detection
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$OS" in
    linux)
        OS_NAME="linux"
        ;;
    darwin)
        OS_NAME="darwin"
        ;;
    *)
        echo "Error: Unsupported operating system: $OS" >&2
        exit 1
        ;;
esac

# Arch detection
ARCH="$(uname -m)"
case "$ARCH" in
    x86_64|amd64)
        ARCH_NAME="x64"
        ;;
    aarch64|arm64)
        ARCH_NAME="arm64"
        ;;
    *)
        echo "Error: Unsupported architecture: $ARCH" >&2
        exit 1
        ;;
esac

ASSET_NAME="telesend-${OS_NAME}-${ARCH_NAME}"

# Determine download URL
if [ "$VERSION" = "latest" ]; then
    DOWNLOAD_URL="$GITHUB_URL/releases/latest/download/$ASSET_NAME"
else
    DOWNLOAD_URL="$GITHUB_URL/releases/download/$VERSION/$ASSET_NAME"
fi

# Create temporary directory
if command -v mktemp >/dev/null 2>&1; then
    TMP_DIR="$(mktemp -d)"
else
    TMP_DIR="/tmp/telesend-install-$$"
    mkdir -p "$TMP_DIR"
fi
trap 'rm -rf "$TMP_DIR"' EXIT HUP INT TERM

echo "Detected OS: $OS_NAME"
echo "Detected Arch: $ARCH_NAME"
echo "Downloading $ASSET_NAME ($VERSION)..."

# Download
if ! curl -fsSL -o "$TMP_DIR/telesend" "$DOWNLOAD_URL"; then
    echo "Error: Failed to download $ASSET_NAME" >&2
    echo "URL attempted: $DOWNLOAD_URL" >&2
    echo "Please check if the version exists and is correct." >&2
    exit 1
fi

# Ensure install dir exists
if [ ! -d "$INSTALL_DIR" ]; then
    echo "Creating directory $INSTALL_DIR..."
    mkdir -p "$INSTALL_DIR"
fi

# Install
echo "Installing to $INSTALL_DIR/telesend..."
mv "$TMP_DIR/telesend" "$INSTALL_DIR/telesend"
chmod +x "$INSTALL_DIR/telesend"

echo ""
echo "Success! Telesend has been installed to:"
echo "  $INSTALL_DIR/telesend"

# PATH check
case ":$PATH:" in
    *":$INSTALL_DIR:"*) ;;
    *)
        echo ""
        echo "Warning: $INSTALL_DIR is not in your PATH."
        echo "You may need to add it to your profile (e.g. ~/.bashrc or ~/.zshrc):"
        echo "  export PATH=\"\$PATH:$INSTALL_DIR\""
        ;;
esac

echo ""
echo "You can now run 'telesend'."
