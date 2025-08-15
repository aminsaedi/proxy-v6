#!/bin/bash

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
REPO="${GITHUB_REPO:-aminsaedi/proxy-v6}"
INSTALL_DIR="/usr/local/bin"
TEMP_DIR=$(mktemp -d)
FORCE_INSTALL=false
CHECK_VERSION=false

# Cleanup on exit
trap "rm -rf $TEMP_DIR" EXIT

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        -f|--force)
            FORCE_INSTALL=true
            shift
            ;;
        -v|--version)
            CHECK_VERSION=true
            shift
            ;;
        -h|--help)
            echo "Usage: $0 [OPTIONS]"
            echo ""
            echo "Options:"
            echo "  -f, --force     Force reinstall even if already installed"
            echo "  -v, --version   Check installed version and compare with latest"
            echo "  -h, --help      Show this help message"
            echo ""
            echo "Environment variables:"
            echo "  GITHUB_REPO     Override the GitHub repository (default: aminsaedi/proxy-v6)"
            echo "  INSTALL_DIR     Override installation directory (default: /usr/local/bin)"
            exit 0
            ;;
        *)
            echo -e "${RED}Unknown option: $1${NC}"
            exit 1
            ;;
    esac
done

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

# Function to get version of installed binary
get_installed_version() {
    local binary=$1
    if [ -f "$INSTALL_DIR/proxy-v6-${binary}" ]; then
        if $INSTALL_DIR/proxy-v6-${binary} version 2>/dev/null | grep -q "Version:"; then
            $INSTALL_DIR/proxy-v6-${binary} version | grep "Version:" | awk '{print $2}'
        else
            echo "unknown"
        fi
    else
        echo "not-installed"
    fi
}

# Check for existing installation
check_existing_installation() {
    local agent_version=$(get_installed_version "agent")
    local coordinator_version=$(get_installed_version "coordinator")
    local monitor_version=$(get_installed_version "monitor")
    
    if [ "$agent_version" != "not-installed" ] || [ "$coordinator_version" != "not-installed" ] || [ "$monitor_version" != "not-installed" ]; then
        echo -e "${BLUE}Current installed versions:${NC}"
        [ "$agent_version" != "not-installed" ] && echo "  Agent:       $agent_version"
        [ "$coordinator_version" != "not-installed" ] && echo "  Coordinator: $coordinator_version"
        [ "$monitor_version" != "not-installed" ] && echo "  Monitor:     $monitor_version"
        return 0
    else
        return 1
    fi
}

# Get latest release
echo "Fetching latest release information..."
LATEST_RELEASE=$(curl -s "https://api.github.com/repos/$REPO/releases/latest" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')

if [ -z "$LATEST_RELEASE" ]; then
    echo -e "${RED}Failed to fetch latest release${NC}"
    exit 1
fi

echo -e "${GREEN}Latest available version: $LATEST_RELEASE${NC}"

# Check version only mode
if [ "$CHECK_VERSION" = true ]; then
    if check_existing_installation; then
        echo ""
        echo -e "${BLUE}Latest available: $LATEST_RELEASE${NC}"
        echo ""
        echo "To upgrade, run: curl -sSL https://raw.githubusercontent.com/$REPO/main/install.sh | bash"
    else
        echo -e "${YELLOW}No proxy-v6 installation found${NC}"
        echo "To install, run: curl -sSL https://raw.githubusercontent.com/$REPO/main/install.sh | bash"
    fi
    exit 0
fi

# Check for existing installation and prompt for upgrade
if check_existing_installation; then
    echo ""
    if [ "$FORCE_INSTALL" != true ]; then
        echo -e "${YELLOW}Proxy-v6 is already installed. Would you like to upgrade to $LATEST_RELEASE? (y/n/f)${NC}"
        echo "  y - Yes, upgrade to latest version"
        echo "  n - No, keep current version"
        echo "  f - Force reinstall"
        read -r response
        case "$response" in
            [yY]|[yY][eE][sS])
                echo -e "${GREEN}Proceeding with upgrade...${NC}"
                ;;
            [fF]|[fF][oO][rR][cC][eE])
                echo -e "${YELLOW}Force reinstalling...${NC}"
                FORCE_INSTALL=true
                ;;
            *)
                echo -e "${BLUE}Installation cancelled${NC}"
                exit 0
                ;;
        esac
    else
        echo -e "${YELLOW}Force reinstalling...${NC}"
    fi
    echo ""
fi

echo -e "${GREEN}Installing proxy-v6 $LATEST_RELEASE for $SUFFIX...${NC}"

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

# Backup existing binaries if upgrading
if check_existing_installation > /dev/null 2>&1; then
    echo "Backing up existing binaries..."
    for binary in agent coordinator monitor; do
        if [ -f "$INSTALL_DIR/proxy-v6-${binary}" ]; then
            $SUDO cp "$INSTALL_DIR/proxy-v6-${binary}" "$INSTALL_DIR/proxy-v6-${binary}.backup" 2>/dev/null || true
        fi
    done
fi

# Install each binary
for binary in agent coordinator monitor; do
    if [ -f "${binary}-${SUFFIX}" ]; then
        $SUDO mv "${binary}-${SUFFIX}" "$INSTALL_DIR/proxy-v6-${binary}"
        $SUDO chmod +x "$INSTALL_DIR/proxy-v6-${binary}"
        
        # Verify the new binary works
        if $INSTALL_DIR/proxy-v6-${binary} version > /dev/null 2>&1; then
            NEW_VERSION=$($INSTALL_DIR/proxy-v6-${binary} version | grep "Version:" | awk '{print $2}')
            echo -e "${GREEN}✓ Installed proxy-v6-${binary} (${NEW_VERSION})${NC}"
            
            # Remove backup if installation successful
            $SUDO rm -f "$INSTALL_DIR/proxy-v6-${binary}.backup" 2>/dev/null || true
        else
            echo -e "${RED}✗ Failed to verify proxy-v6-${binary}${NC}"
            
            # Restore backup if available
            if [ -f "$INSTALL_DIR/proxy-v6-${binary}.backup" ]; then
                echo -e "${YELLOW}  Restoring previous version...${NC}"
                $SUDO mv "$INSTALL_DIR/proxy-v6-${binary}.backup" "$INSTALL_DIR/proxy-v6-${binary}"
            fi
        fi
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
echo "Commands available:"
echo "  proxy-v6-agent version       - Check agent version"
echo "  proxy-v6-coordinator version - Check coordinator version"
echo "  proxy-v6-monitor version     - Check monitor version"
echo ""
echo "Usage:"
echo "  Start agent:       proxy-v6-agent --coordinator http://coordinator:8081"
echo "  Start coordinator: proxy-v6-coordinator --proxy-port 8888"
echo "  Start monitor:     proxy-v6-monitor --coordinator http://coordinator:8081"
echo ""
echo "Or use short aliases:"
echo "  pv6-agent, pv6-coordinator, pv6-monitor"
echo ""
echo "To check for updates later, run:"
echo "  curl -sSL https://raw.githubusercontent.com/$REPO/main/install.sh | bash -s -- --version"