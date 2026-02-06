#!/bin/bash
set -e

REPO="mart337i/odooctl"
INSTALL_DIR="/usr/local/bin"
BINARY_NAME="odooctl"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

error() {
    echo -e "${RED}[ERROR]${NC} $1"
    exit 1
}

# Detect OS and architecture
detect_platform() {
    OS=$(uname -s | tr '[:upper:]' '[:lower:]')
    ARCH=$(uname -m)

    case "$OS" in
        linux)
            OS="linux"
            ;;
        darwin)
            OS="darwin"
            ;;
        mingw*|msys*|cygwin*)
            OS="windows"
            ;;
        *)
            error "Unsupported operating system: $OS"
            ;;
    esac

    case "$ARCH" in
        x86_64|amd64)
            ARCH="amd64"
            ;;
        aarch64|arm64)
            ARCH="arm64"
            ;;
        *)
            error "Unsupported architecture: $ARCH"
            ;;
    esac

    PLATFORM="${OS}-${ARCH}"
    info "Detected platform: $PLATFORM"
}

# Get latest release version
get_latest_version() {
    info "Fetching latest release..."
    VERSION=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')
    
    if [ -z "$VERSION" ]; then
        error "Failed to fetch latest version"
    fi
    
    info "Latest version: $VERSION"
}

# Download and install
install() {
    BINARY="odooctl-${PLATFORM}"
    if [ "$OS" = "windows" ]; then
        BINARY="${BINARY}.exe"
    fi

    DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${VERSION}/${BINARY}"
    
    info "Downloading $BINARY..."
    
    TMP_DIR=$(mktemp -d)
    TMP_FILE="${TMP_DIR}/${BINARY_NAME}"
    
    if ! curl -fsSL "$DOWNLOAD_URL" -o "$TMP_FILE"; then
        rm -rf "$TMP_DIR"
        error "Failed to download from $DOWNLOAD_URL"
    fi
    
    # Download and verify checksum if available
    CHECKSUM_URL="https://github.com/${REPO}/releases/download/${VERSION}/checksums.txt"
    if curl -fsSL "$CHECKSUM_URL" -o "${TMP_DIR}/checksums.txt" 2>/dev/null; then
        info "Verifying checksum..."
        
        # Extract expected checksum for this binary
        EXPECTED=$(grep "$BINARY" "${TMP_DIR}/checksums.txt" | awk '{print $1}')
        
        if [ -n "$EXPECTED" ]; then
            # Calculate actual checksum
            if command -v sha256sum &> /dev/null; then
                ACTUAL=$(sha256sum "$TMP_FILE" | awk '{print $1}')
            elif command -v shasum &> /dev/null; then
                ACTUAL=$(shasum -a 256 "$TMP_FILE" | awk '{print $1}')
            else
                warn "No sha256sum or shasum available, skipping checksum verification"
                ACTUAL="$EXPECTED"
            fi
            
            if [ "$EXPECTED" != "$ACTUAL" ]; then
                rm -rf "$TMP_DIR"
                error "Checksum mismatch! Expected: $EXPECTED, Got: $ACTUAL"
            fi
            info "Checksum verified successfully"
        else
            warn "Checksum not found for $BINARY, skipping verification"
        fi
    else
        warn "Checksums file not available for this release, skipping verification"
    fi
    
    chmod +x "$TMP_FILE"
    
    # Install to system directory (requires sudo on Linux/macOS)
    if [ "$OS" != "windows" ]; then
        if [ -w "$INSTALL_DIR" ]; then
            mv "$TMP_FILE" "${INSTALL_DIR}/${BINARY_NAME}"
        else
            info "Installing to $INSTALL_DIR (requires sudo)..."
            sudo mv "$TMP_FILE" "${INSTALL_DIR}/${BINARY_NAME}"
        fi
    else
        mv "$TMP_FILE" "${INSTALL_DIR}/${BINARY_NAME}.exe"
    fi
    
    rm -rf "$TMP_DIR"
    
    info "Installed $BINARY_NAME to ${INSTALL_DIR}"
}

# Verify installation
verify() {
    if command -v "$BINARY_NAME" &> /dev/null; then
        INSTALLED_VERSION=$($BINARY_NAME version 2>/dev/null || echo "unknown")
        info "Successfully installed: $INSTALLED_VERSION"
    else
        warn "$BINARY_NAME installed but not in PATH. Add $INSTALL_DIR to your PATH."
    fi
}

main() {
    echo ""
    echo "  odooctl installer"
    echo "  ================="
    echo ""
    
    detect_platform
    get_latest_version
    install
    verify
    
    echo ""
    info "Installation complete! Run 'odooctl --help' to get started."
    echo ""
}

main
