#!/bin/bash
# Installation script for syncnorris (Linux/macOS)

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Configuration
REPO="sdejongh/syncnorris"
BINARY_NAME="syncnorris"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"

# Detect OS and architecture
detect_platform() {
    local os=$(uname -s | tr '[:upper:]' '[:lower:]')
    local arch=$(uname -m)

    case "$os" in
        linux*)
            OS="Linux"
            ;;
        darwin*)
            OS="Darwin"
            ;;
        *)
            echo -e "${RED}Error: Unsupported operating system: $os${NC}"
            echo "This script supports Linux and macOS only."
            echo "For Windows, please use install.ps1 with PowerShell."
            exit 1
            ;;
    esac

    case "$arch" in
        x86_64|amd64)
            ARCH="x86_64"
            ;;
        aarch64|arm64)
            ARCH="arm64"
            ;;
        *)
            echo -e "${RED}Error: Unsupported architecture: $arch${NC}"
            exit 1
            ;;
    esac

    echo -e "${GREEN}Detected platform: ${OS}_${ARCH}${NC}"
}

# Get latest release version
get_latest_version() {
    echo -e "${YELLOW}Fetching latest release...${NC}"

    # Try using curl first, fall back to wget
    if command -v curl &> /dev/null; then
        VERSION=$(curl -s "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')
    elif command -v wget &> /dev/null; then
        VERSION=$(wget -qO- "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')
    else
        echo -e "${RED}Error: Neither curl nor wget is available${NC}"
        echo "Please install curl or wget and try again."
        exit 1
    fi

    if [ -z "$VERSION" ]; then
        echo -e "${RED}Error: Could not fetch latest version${NC}"
        exit 1
    fi

    echo -e "${GREEN}Latest version: ${VERSION}${NC}"
}

# Download and extract archive
download_and_extract() {
    local archive_name="${BINARY_NAME}_${VERSION}_${OS}_${ARCH}.tar.gz"
    local download_url="https://github.com/${REPO}/releases/download/${VERSION}/${archive_name}"
    local tmp_dir=$(mktemp -d)

    echo -e "${YELLOW}Downloading ${archive_name}...${NC}"

    if command -v curl &> /dev/null; then
        curl -sL "$download_url" -o "${tmp_dir}/${archive_name}"
    else
        wget -q "$download_url" -O "${tmp_dir}/${archive_name}"
    fi

    if [ $? -ne 0 ]; then
        echo -e "${RED}Error: Download failed${NC}"
        echo "URL: $download_url"
        rm -rf "$tmp_dir"
        exit 1
    fi

    echo -e "${YELLOW}Extracting archive...${NC}"
    tar -xzf "${tmp_dir}/${archive_name}" -C "$tmp_dir"

    if [ $? -ne 0 ]; then
        echo -e "${RED}Error: Extraction failed${NC}"
        rm -rf "$tmp_dir"
        exit 1
    fi

    BINARY_PATH="${tmp_dir}/${BINARY_NAME}"

    if [ ! -f "$BINARY_PATH" ]; then
        echo -e "${RED}Error: Binary not found in archive${NC}"
        rm -rf "$tmp_dir"
        exit 1
    fi
}

# Install binary
install_binary() {
    echo -e "${YELLOW}Installing to ${INSTALL_DIR}...${NC}"

    # Check if install directory exists and is writable
    if [ ! -d "$INSTALL_DIR" ]; then
        echo -e "${YELLOW}Creating directory ${INSTALL_DIR}...${NC}"
        sudo mkdir -p "$INSTALL_DIR"
    fi

    # Try to install with sudo if needed
    if [ -w "$INSTALL_DIR" ]; then
        cp "$BINARY_PATH" "${INSTALL_DIR}/${BINARY_NAME}"
        chmod +x "${INSTALL_DIR}/${BINARY_NAME}"
    else
        echo -e "${YELLOW}Requesting sudo privileges for installation...${NC}"
        sudo cp "$BINARY_PATH" "${INSTALL_DIR}/${BINARY_NAME}"
        sudo chmod +x "${INSTALL_DIR}/${BINARY_NAME}"
    fi

    if [ $? -ne 0 ]; then
        echo -e "${RED}Error: Installation failed${NC}"
        exit 1
    fi

    # Cleanup
    rm -rf "$tmp_dir"

    echo -e "${GREEN}✓ Successfully installed ${BINARY_NAME} to ${INSTALL_DIR}${NC}"
}

# Verify installation
verify_installation() {
    if command -v "$BINARY_NAME" &> /dev/null; then
        local installed_version=$("$BINARY_NAME" --version 2>&1 | head -n 1)
        echo -e "${GREEN}✓ Installation verified${NC}"
        echo -e "${GREEN}  $installed_version${NC}"
    else
        echo -e "${YELLOW}Warning: ${BINARY_NAME} is installed but not in PATH${NC}"
        echo -e "You may need to add ${INSTALL_DIR} to your PATH:"
        echo -e "  export PATH=\"\$PATH:${INSTALL_DIR}\""
    fi
}

# Main installation process
main() {
    echo ""
    echo "========================================="
    echo "  syncnorris Installation Script"
    echo "========================================="
    echo ""

    detect_platform
    get_latest_version
    download_and_extract
    install_binary
    verify_installation

    echo ""
    echo -e "${GREEN}Installation complete!${NC}"
    echo ""
    echo "Run '${BINARY_NAME} --help' to get started."
    echo ""
}

main
