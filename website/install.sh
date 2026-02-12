#!/bin/sh
# TuTu Installer — Linux & macOS
# Usage: curl -fsSL https://tutuengine.tech/install | sh
set -e

REPO="NikeGunn/tutu"
BINARY="tutu"
INSTALL_DIR="/usr/local/bin"

# ─── Detect Platform ────────────────────────────────────────────────────
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

case "$ARCH" in
    x86_64|amd64)  ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *)
        echo "Error: Unsupported architecture: $ARCH"
        echo "TuTu supports x86_64 (amd64) and aarch64 (arm64)."
        exit 1
        ;;
esac

case "$OS" in
    linux|darwin) ;;
    *)
        echo "Error: Unsupported OS: $OS"
        echo "Use 'winget install tutu-network.tutu' on Windows."
        exit 1
        ;;
esac

# ─── Get Latest Release ─────────────────────────────────────────────────
echo ""
echo "  ████████╗██╗   ██╗████████╗██╗   ██╗"
echo "  ╚══██╔══╝██║   ██║╚══██╔══╝██║   ██║"
echo "     ██║   ██║   ██║   ██║   ██║   ██║"
echo "     ██║   ╚██████╔╝   ██║   ╚██████╔╝"
echo "     ╚═╝    ╚═════╝    ╚═╝    ╚═════╝"
echo ""
echo "  Installing TuTu for ${OS}/${ARCH}..."
echo ""

# Determine download URL
VERSION=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" 2>/dev/null | grep '"tag_name"' | head -1 | sed -E 's/.*"tag_name": *"([^"]+)".*/\1/' || echo "")

if [ -z "$VERSION" ]; then
    echo "  Could not detect latest version. Downloading from main branch..."
    URL="https://github.com/${REPO}/releases/download/v0.1.0/tutu-${OS}-${ARCH}"
else
    echo "  Latest version: ${VERSION}"
    URL="https://github.com/${REPO}/releases/download/${VERSION}/tutu-${OS}-${ARCH}"
fi

# ─── Download ────────────────────────────────────────────────────────────
TMPFILE=$(mktemp)
echo "  Downloading ${URL}..."

if command -v curl >/dev/null 2>&1; then
    curl -fSL "$URL" -o "$TMPFILE"
elif command -v wget >/dev/null 2>&1; then
    wget -q "$URL" -O "$TMPFILE"
else
    echo "Error: curl or wget required."
    exit 1
fi

chmod +x "$TMPFILE"

# ─── Install ─────────────────────────────────────────────────────────────
if [ -w "$INSTALL_DIR" ]; then
    mv "$TMPFILE" "${INSTALL_DIR}/${BINARY}"
else
    echo "  Requires elevated permissions to install to ${INSTALL_DIR}..."
    sudo mv "$TMPFILE" "${INSTALL_DIR}/${BINARY}"
fi

# ─── Verify ──────────────────────────────────────────────────────────────
if command -v tutu >/dev/null 2>&1; then
    INSTALLED_VERSION=$(tutu --version 2>/dev/null || echo "unknown")
    echo ""
    echo "  ✅ TuTu installed successfully! (${INSTALLED_VERSION})"
    echo ""
    echo "  Get started:"
    echo "    tutu run llama3.2        # Chat with Llama 3.2"
    echo "    tutu run phi3            # Chat with Phi-3"
    echo "    tutu serve               # Start API server"
    echo "    tutu --help              # See all commands"
    echo ""
    echo "  Documentation: https://tutuengine.tech/docs"
    echo ""
else
    echo ""
    echo "  ⚠️  TuTu was downloaded but might not be in your PATH."
    echo "     Binary location: ${INSTALL_DIR}/${BINARY}"
    echo "     Add ${INSTALL_DIR} to your PATH if needed."
    echo ""
fi
