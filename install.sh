#!/bin/bash

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Configuration
REPO="${GITHUB_REPO:-aminsaedi/proxy-v6}"
INSTALL_DIR="/usr/local/bin"
TEMP_DIR=$(mktemp -d)

# Cleanup on exit
trap "rm -rf $TEMP_DIR" EXIT

# Detect OS and Architecture
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

case "$OS" in
    linux)
        PLATFORM="linux"
        ;;
    darwin)
        PLATFORM="darwin"
        ;;
    *)
        echo -e "${RED}Unsupported operating system: $OS${NC}"
        exit 1
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
        echo -e "${RED}Unsupported architecture: $ARCH${NC}"
        exit 1
        ;;
esac

SUFFIX="${PLATFORM}-${ARCH}"

echo -e "${GREEN}Installing proxy-v6 for $SUFFIX...${NC}"

# Get latest release
echo "Fetching latest release..."
LATEST_RELEASE=$(curl -s "https://api.github.com/repos/$REPO/releases/latest" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')

if [ -z "$LATEST_RELEASE" ]; then
    echo -e "${RED}Failed to fetch latest release${NC}"
    exit 1
fi

echo -e "${GREEN}Latest version: $LATEST_RELEASE${NC}"

# Download release
DOWNLOAD_URL="https://github.com/$REPO/releases/download/$LATEST_RELEASE/proxy-v6-$SUFFIX.tar.gz"
echo "Downloading from $DOWNLOAD_URL..."

cd "$TEMP_DIR"
if ! curl -sSL -o "proxy-v6-$SUFFIX.tar.gz" "$DOWNLOAD_URL"; then
    echo -e "${RED}Failed to download release${NC}"
    exit 1
fi

# Extract
echo "Extracting binaries..."
tar xzf "proxy-v6-$SUFFIX.tar.gz"

# Install binaries
echo "Installing binaries to $INSTALL_DIR..."

# Check if we need sudo
if [ -w "$INSTALL_DIR" ]; then
    SUDO=""
else
    SUDO="sudo"
    echo -e "${YELLOW}Root privileges required for installation${NC}"
fi

# Install each binary
for binary in agent coordinator monitor; do
    if [ -f "${binary}-${SUFFIX}" ]; then
        $SUDO mv "${binary}-${SUFFIX}" "$INSTALL_DIR/proxy-v6-${binary}"
        $SUDO chmod +x "$INSTALL_DIR/proxy-v6-${binary}"
        echo -e "${GREEN}✓ Installed proxy-v6-${binary}${NC}"
    else
        echo -e "${YELLOW}⚠ ${binary} not found in release${NC}"
    fi
done

# Create convenience symlinks
echo "Creating convenience commands..."
$SUDO ln -sf "$INSTALL_DIR/proxy-v6-agent" "$INSTALL_DIR/pv6-agent" 2>/dev/null || true
$SUDO ln -sf "$INSTALL_DIR/proxy-v6-coordinator" "$INSTALL_DIR/pv6-coordinator" 2>/dev/null || true
$SUDO ln -sf "$INSTALL_DIR/proxy-v6-monitor" "$INSTALL_DIR/pv6-monitor" 2>/dev/null || true

# Check if tinyproxy is installed
if ! command -v tinyproxy &> /dev/null; then
    echo -e "${YELLOW}Warning: tinyproxy is not installed${NC}"
    echo "To install tinyproxy:"
    case "$PLATFORM" in
        linux)
            if [ -f /etc/debian_version ]; then
                echo "  sudo apt-get install tinyproxy"
            elif [ -f /etc/redhat-release ]; then
                echo "  sudo yum install tinyproxy"
            else
                echo "  Please install tinyproxy using your package manager"
            fi
            ;;
        darwin)
            echo "  brew install tinyproxy"
            ;;
    esac
fi

echo -e "${GREEN}Installation complete!${NC}"
echo ""
echo "Usage:"
echo "  Start agent:       proxy-v6-agent --coordinator http://coordinator:8081"
echo "  Start coordinator: proxy-v6-coordinator --proxy-port 8888"
echo "  Start monitor:     proxy-v6-monitor --coordinator http://coordinator:8081"
echo ""
echo "Or use short aliases:"
echo "  pv6-agent, pv6-coordinator, pv6-monitor"