#!/bin/sh
# TuTu Installer — Linux & macOS
# Usage: curl -fsSL https://tutuengine.tech/install.sh | sh
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
    armv7l)        ARCH="arm" ;;
    *)
        echo "  Error: Unsupported architecture: $ARCH"
        echo "  TuTu supports x86_64 (amd64), aarch64 (arm64), and armv7l."
        exit 1
        ;;
esac

case "$OS" in
    linux|darwin) ;;
    *)
        echo "  Error: Unsupported OS: $OS"
        echo "  Use 'irm tutuengine.tech/install.ps1 | iex' on Windows."
        exit 1
        ;;
esac

# ─── Banner ──────────────────────────────────────────────────────────────
echo ""
echo "  ████████╗██╗   ██╗████████╗██╗   ██╗"
echo "  ╚══██╔══╝██║   ██║╚══██╔══╝██║   ██║"
echo "     ██║   ██║   ██║   ██║   ██║   ██║"
echo "     ██║   ╚██████╔╝   ██║   ╚██████╔╝"
echo "     ╚═╝    ╚═════╝    ╚═╝    ╚═════╝"
echo ""
echo "  Installing TuTu for ${OS}/${ARCH}..."
echo ""

# ─── Get Latest Release ─────────────────────────────────────────────────
VERSION=""
if command -v curl >/dev/null 2>&1; then
    VERSION=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" 2>/dev/null \
        | grep '"tag_name"' | head -1 \
        | sed -E 's/.*"tag_name": *"([^"]+)".*/\1/' || echo "")
elif command -v wget >/dev/null 2>&1; then
    VERSION=$(wget -qO- "https://api.github.com/repos/${REPO}/releases/latest" 2>/dev/null \
        | grep '"tag_name"' | head -1 \
        | sed -E 's/.*"tag_name": *"([^"]+)".*/\1/' || echo "")
fi

if [ -z "$VERSION" ]; then
    VERSION="v0.1.0"
    echo "  Using default version: ${VERSION}"
else
    echo "  Latest version: ${VERSION}"
fi

URL="https://github.com/${REPO}/releases/download/${VERSION}/tutu-${OS}-${ARCH}"

# ─── Download ────────────────────────────────────────────────────────────
TMPFILE=$(mktemp)
echo "  Downloading ${URL}..."

download_success=false
if command -v curl >/dev/null 2>&1; then
    if curl -fSL --connect-timeout 15 --retry 3 "$URL" -o "$TMPFILE" 2>/dev/null; then
        download_success=true
    fi
elif command -v wget >/dev/null 2>&1; then
    if wget -q --timeout=15 --tries=3 "$URL" -O "$TMPFILE" 2>/dev/null; then
        download_success=true
    fi
else
    echo "  Error: curl or wget is required."
    exit 1
fi

if [ "$download_success" = false ]; then
    rm -f "$TMPFILE"
    echo ""
    echo "  ⚠  Pre-built binary not available for ${OS}/${ARCH} (${VERSION})."
    echo ""
    echo "  Build from source instead (requires Go 1.24+):"
    echo ""
    echo "    git clone https://github.com/${REPO}.git"
    echo "    cd tutu"
    echo "    go build -o tutu ./cmd/tutu"
    echo "    sudo mv tutu ${INSTALL_DIR}/tutu"
    echo ""
    echo "  Or check releases: https://github.com/${REPO}/releases"
    echo ""
    exit 1
fi

chmod +x "$TMPFILE"

# ─── Validate binary ────────────────────────────────────────────────────
if ! file "$TMPFILE" 2>/dev/null | grep -qi "executable\|ELF\|Mach-O"; then
    # Basic sanity check — the downloaded file should be an executable, not HTML
    if head -c 20 "$TMPFILE" 2>/dev/null | grep -qi "<!DOCTYPE\|<html\|Not Found"; then
        rm -f "$TMPFILE"
        echo ""
        echo "  ⚠  Download failed — received HTML instead of binary."
        echo "     The release ${VERSION} may not exist for ${OS}/${ARCH}."
        echo ""
        echo "  Build from source instead (requires Go 1.24+):"
        echo ""
        echo "    git clone https://github.com/${REPO}.git"
        echo "    cd tutu"
        echo "    go build -o tutu ./cmd/tutu"
        echo "    sudo mv tutu ${INSTALL_DIR}/tutu"
        echo ""
        exit 1
    fi
fi

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
    echo "  Documentation: https://tutuengine.tech/docs.html"
    echo ""
else
    echo ""
    echo "  ⚠  TuTu was downloaded but may not be in your PATH."
    echo "     Binary location: ${INSTALL_DIR}/${BINARY}"
    echo "     Add ${INSTALL_DIR} to your PATH if needed."
    echo ""
fi
