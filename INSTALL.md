# Installation Scripts Documentation

This document describes the automated installation scripts for syncnorris.

## Overview

syncnorris provides platform-specific installation scripts that automatically download and install the latest release:

- **`install.sh`**: For Linux and macOS (Bash script)
- **`install.ps1`**: For Windows (PowerShell script)

## Linux & macOS Installation (`install.sh`)

### Usage

**One-line installation:**
```bash
curl -sSL https://raw.githubusercontent.com/sdejongh/syncnorris/master/install.sh | bash
```

**Or with wget:**
```bash
wget -qO- https://raw.githubusercontent.com/sdejongh/syncnorris/master/install.sh | bash
```

**Custom installation directory:**
```bash
curl -sSL https://raw.githubusercontent.com/sdejongh/syncnorris/master/install.sh | INSTALL_DIR="$HOME/.local/bin" bash
```

### How It Works

1. **Platform Detection**: Automatically detects OS (Linux/Darwin) and architecture (x86_64/arm64)
2. **Version Fetching**: Queries GitHub API for the latest release
3. **Download**: Downloads the appropriate `.tar.gz` archive
4. **Extraction**: Extracts the binary from the archive
5. **Installation**: Copies binary to `/usr/local/bin` (or custom `$INSTALL_DIR`)
6. **Verification**: Confirms installation and displays version

### Requirements

- Bash shell
- `curl` or `wget` (at least one must be installed)
- `tar` (standard on all Unix systems)
- `sudo` access if installing to `/usr/local/bin`

### Environment Variables

- `INSTALL_DIR`: Custom installation directory (default: `/usr/local/bin`)

### Exit Codes

- `0`: Success
- `1`: Error (unsupported platform, download failed, etc.)

## Windows Installation (`install.ps1`)

### Usage

**One-line installation (PowerShell 3.0+):**
```powershell
irm https://raw.githubusercontent.com/sdejongh/syncnorris/master/install.ps1 | iex
```

**Download and run:**
```powershell
# Download
Invoke-WebRequest -Uri https://raw.githubusercontent.com/sdejongh/syncnorris/master/install.ps1 -OutFile install.ps1

# Run
powershell -ExecutionPolicy Bypass -File install.ps1
```

### How It Works

1. **Architecture Detection**: Detects x86_64 or ARM64
2. **Version Fetching**: Queries GitHub API for the latest release
3. **Download**: Downloads the appropriate `.zip` archive
4. **Extraction**: Extracts using built-in `Expand-Archive`
5. **Installation**: Copies binary to `%LOCALAPPDATA%\syncnorris`
6. **PATH Update**: Adds installation directory to user PATH
7. **Verification**: Confirms installation and displays version

### Requirements

- Windows PowerShell 5.0+ (included with Windows 10/11)
- Internet connection
- No administrator privileges required (installs to user directory)

### Installation Location

- Binary: `%LOCALAPPDATA%\syncnorris\syncnorris.exe`
- Typically: `C:\Users\<username>\AppData\Local\syncnorris\syncnorris.exe`

### PATH Configuration

The script automatically adds the installation directory to the **user PATH** (not system PATH), so no administrator privileges are required.

**Note**: You may need to restart your terminal/PowerShell for PATH changes to take effect.

## Testing the Scripts

### Local Testing (Linux/macOS)

```bash
# Test the script locally
bash install.sh

# Test with custom directory
INSTALL_DIR="$HOME/bin" bash install.sh
```

### Local Testing (Windows)

```powershell
# Run the local script
powershell -ExecutionPolicy Bypass -File install.ps1
```

## Troubleshooting

### Linux/macOS

**"Neither curl nor wget is available"**
```bash
# Ubuntu/Debian
sudo apt-get install curl

# CentOS/RHEL
sudo yum install curl

# macOS
# curl is pre-installed
```

**"Permission denied" when installing**
- The script will automatically request sudo privileges
- Make sure your user has sudo access
- Or specify a custom `INSTALL_DIR` you have write access to:
  ```bash
  INSTALL_DIR="$HOME/.local/bin" bash install.sh
  ```

**"command not found" after installation**
- Check if `/usr/local/bin` is in your PATH:
  ```bash
  echo $PATH
  ```
- If not, add it to your shell profile:
  ```bash
  # For bash
  echo 'export PATH="$PATH:/usr/local/bin"' >> ~/.bashrc
  source ~/.bashrc

  # For zsh
  echo 'export PATH="$PATH:/usr/local/bin"' >> ~/.zshrc
  source ~/.zshrc
  ```

### Windows

**"Execution Policy" error**
```powershell
# Temporarily bypass execution policy
Set-ExecutionPolicy -ExecutionPolicy Bypass -Scope Process
# Then run the script
.\install.ps1
```

**"Cannot find syncnorris command" after installation**
1. Restart your PowerShell/terminal
2. Or manually add to PATH for current session:
   ```powershell
   $env:Path += ";$env:LOCALAPPDATA\syncnorris"
   ```

**Download fails with SSL/TLS error**
```powershell
# Enable TLS 1.2
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
# Then run the installer again
```

## Updating

The installation scripts always download the **latest release**. To update syncnorris, simply run the installation script again:

```bash
# Linux/macOS
curl -sSL https://raw.githubusercontent.com/sdejongh/syncnorris/master/install.sh | bash

# Windows
irm https://raw.githubusercontent.com/sdejongh/syncnorris/master/install.ps1 | iex
```

## Uninstallation

### Linux/macOS

```bash
# If installed to default location
sudo rm /usr/local/bin/syncnorris

# If installed to custom location
rm $INSTALL_DIR/syncnorris
```

### Windows

```powershell
# Remove binary
Remove-Item "$env:LOCALAPPDATA\syncnorris" -Recurse -Force

# Remove from PATH (manual)
# Go to: System Properties → Environment Variables → User Variables → Path
# Remove the entry: %LOCALAPPDATA%\syncnorris
```

## Security Considerations

**Piping scripts to bash/PowerShell:**

The one-line installation method (`curl ... | bash`) is convenient but executes code directly from the internet. For enhanced security:

1. **Review the script first:**
   ```bash
   curl -sSL https://raw.githubusercontent.com/sdejongh/syncnorris/master/install.sh
   ```

2. **Download and inspect before running:**
   ```bash
   curl -sSL https://raw.githubusercontent.com/sdejongh/syncnorris/master/install.sh -o install.sh
   # Review the script
   less install.sh
   # Then run it
   bash install.sh
   ```

3. **Verify checksums** (manual installation):
   - Download `checksums.txt` from the release page
   - Verify the archive checksum before extracting

## Contributing

To improve the installation scripts:

1. Test changes on all supported platforms
2. Ensure error handling is robust
3. Update this documentation
4. Submit a pull request
