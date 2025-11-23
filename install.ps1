# Installation script for syncnorris (Windows PowerShell)
# Run this script with: powershell -ExecutionPolicy Bypass -File install.ps1

$ErrorActionPreference = "Stop"

# Configuration
$Repo = "sdejongh/syncnorris"
$BinaryName = "syncnorris.exe"
$InstallDir = "$env:LOCALAPPDATA\syncnorris"

# Colors for output
function Write-ColorOutput {
    param(
        [Parameter(Mandatory=$true)]
        [string]$Message,
        [string]$Color = "White"
    )
    Write-Host $Message -ForegroundColor $Color
}

# Detect architecture
function Get-Architecture {
    $arch = $env:PROCESSOR_ARCHITECTURE
    switch ($arch) {
        "AMD64" { return "x86_64" }
        "ARM64" { return "arm64" }
        default {
            Write-ColorOutput "Error: Unsupported architecture: $arch" "Red"
            exit 1
        }
    }
}

# Get latest release version
function Get-LatestVersion {
    Write-ColorOutput "Fetching latest release..." "Yellow"

    try {
        $apiUrl = "https://api.github.com/repos/$Repo/releases/latest"
        $response = Invoke-RestMethod -Uri $apiUrl -Method Get
        $version = $response.tag_name

        if ([string]::IsNullOrEmpty($version)) {
            throw "Version not found in API response"
        }

        Write-ColorOutput "Latest version: $version" "Green"
        return $version
    }
    catch {
        Write-ColorOutput "Error: Could not fetch latest version" "Red"
        Write-ColorOutput $_.Exception.Message "Red"
        exit 1
    }
}

# Download and extract archive
function Install-Syncnorris {
    param(
        [string]$Version,
        [string]$Arch
    )

    # Remove 'v' prefix from version for filename (GoReleaser uses version without 'v')
    $versionNumber = $Version -replace '^v', ''
    $archiveName = "syncnorris_${versionNumber}_Windows_${Arch}.zip"
    $downloadUrl = "https://github.com/$Repo/releases/download/$Version/$archiveName"
    $tempDir = [System.IO.Path]::GetTempPath()
    $archivePath = Join-Path $tempDir $archiveName
    $extractDir = Join-Path $tempDir "syncnorris_extract"

    Write-ColorOutput "Downloading $archiveName..." "Yellow"

    try {
        # Download archive
        Invoke-WebRequest -Uri $downloadUrl -OutFile $archivePath -UseBasicParsing

        # Create extraction directory
        if (Test-Path $extractDir) {
            Remove-Item $extractDir -Recurse -Force
        }
        New-Item -ItemType Directory -Path $extractDir | Out-Null

        Write-ColorOutput "Extracting archive..." "Yellow"

        # Extract archive
        Expand-Archive -Path $archivePath -DestinationPath $extractDir -Force

        # Find binary
        $binaryPath = Join-Path $extractDir $BinaryName
        if (-not (Test-Path $binaryPath)) {
            throw "Binary not found in archive"
        }

        # Create install directory if it doesn't exist
        if (-not (Test-Path $InstallDir)) {
            Write-ColorOutput "Creating directory $InstallDir..." "Yellow"
            New-Item -ItemType Directory -Path $InstallDir | Out-Null
        }

        # Copy binary to install directory
        $targetPath = Join-Path $InstallDir $BinaryName
        Write-ColorOutput "Installing to $InstallDir..." "Yellow"
        Copy-Item $binaryPath $targetPath -Force

        # Cleanup
        Remove-Item $archivePath -Force
        Remove-Item $extractDir -Recurse -Force

        Write-ColorOutput "✓ Successfully installed syncnorris to $InstallDir" "Green"

        return $targetPath
    }
    catch {
        Write-ColorOutput "Error during installation: $($_.Exception.Message)" "Red"

        # Cleanup on error
        if (Test-Path $archivePath) { Remove-Item $archivePath -Force }
        if (Test-Path $extractDir) { Remove-Item $extractDir -Recurse -Force }

        exit 1
    }
}

# Add to PATH if not already present
function Add-ToPath {
    $currentPath = [Environment]::GetEnvironmentVariable("Path", "User")

    if ($currentPath -notlike "*$InstallDir*") {
        Write-ColorOutput "Adding $InstallDir to user PATH..." "Yellow"

        $newPath = "$currentPath;$InstallDir"
        [Environment]::SetEnvironmentVariable("Path", $newPath, "User")

        # Update current session PATH
        $env:Path = "$env:Path;$InstallDir"

        Write-ColorOutput "✓ Added to PATH" "Green"
        Write-ColorOutput "Note: You may need to restart your terminal for PATH changes to take effect" "Yellow"
    }
    else {
        Write-ColorOutput "✓ Install directory already in PATH" "Green"
    }
}

# Verify installation
function Test-Installation {
    param([string]$BinaryPath)

    try {
        $version = & $BinaryPath --version 2>&1 | Select-Object -First 1
        Write-ColorOutput "✓ Installation verified" "Green"
        Write-ColorOutput "  $version" "Green"
    }
    catch {
        Write-ColorOutput "Warning: Could not verify installation" "Yellow"
    }
}

# Main installation process
function Main {
    Write-Host ""
    Write-Host "=========================================" -ForegroundColor Cyan
    Write-Host "  syncnorris Installation Script" -ForegroundColor Cyan
    Write-Host "=========================================" -ForegroundColor Cyan
    Write-Host ""

    $arch = Get-Architecture
    Write-ColorOutput "Detected architecture: $arch" "Green"

    $version = Get-LatestVersion
    $binaryPath = Install-Syncnorris -Version $version -Arch $arch
    Add-ToPath
    Test-Installation -BinaryPath $binaryPath

    Write-Host ""
    Write-ColorOutput "Installation complete!" "Green"
    Write-Host ""
    Write-Host "Run 'syncnorris --help' to get started."
    Write-Host ""
    Write-Host "If the command is not found, please:"
    Write-Host "  1. Restart your terminal/PowerShell"
    Write-Host "  2. Or run: `$env:Path += ';$InstallDir'"
    Write-Host ""
}

# Run main function
Main
