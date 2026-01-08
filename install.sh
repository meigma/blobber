#!/bin/sh
# Blobber Installer Script
# Usage: curl -fsSL https://blobber.meigma.dev/install.sh | sh
#
# Environment variables:
#   BLOBBER_VERSION    - Version to install (default: latest)
#   BLOBBER_INSTALL    - Installation directory (default: ~/.local/bin)
#   BLOBBER_TMPDIR     - Parent directory for temporary files (default: system temp)
#                        A subdirectory will be created and cleaned up on exit
#   BLOBBER_NO_VERIFY  - Skip signature verification if set to "1"
#
# Examples:
#   # Install latest version
#   curl -fsSL https://blobber.meigma.dev/install.sh | sh
#
#   # Install specific version
#   curl -fsSL https://blobber.meigma.dev/install.sh | BLOBBER_VERSION=1.0.0 sh
#
#   # Install to custom directory
#   curl -fsSL https://blobber.meigma.dev/install.sh | BLOBBER_INSTALL=/usr/local/bin sh

set -e

# Configuration
GITHUB_OWNER="meigma"
GITHUB_REPO="blobber"
BINARY_NAME="blobber"

# Cosign verification parameters (keyless signing via GitHub Actions)
# Identity is scoped to the release workflow and tag refs only
COSIGN_CERT_IDENTITY_REGEXP="https://github.com/meigma/blobber/.github/workflows/release.yml@refs/tags/.*"
COSIGN_OIDC_ISSUER="https://token.actions.githubusercontent.com"

# Colors (disabled if not a terminal)
if [ -t 1 ]; then
    RED='\033[0;31m'
    GREEN='\033[0;32m'
    YELLOW='\033[0;33m'
    BLUE='\033[0;34m'
    BOLD='\033[1m'
    NC='\033[0m' # No Color
else
    RED=''
    GREEN=''
    YELLOW=''
    BLUE=''
    BOLD=''
    NC=''
fi

# Logging functions
log_info() {
    printf "${BLUE}==>${NC} %s\n" "$1"
}

log_success() {
    printf "${GREEN}==>${NC} %s\n" "$1"
}

log_warn() {
    printf "${YELLOW}warning:${NC} %s\n" "$1" >&2
}

log_error() {
    printf "${RED}error:${NC} %s\n" "$1" >&2
}

# Check if a command exists
has_command() {
    command -v "$1" >/dev/null 2>&1
}

# Detect the operating system
detect_os() {
    os="$(uname -s | tr '[:upper:]' '[:lower:]')"
    case "$os" in
        linux)
            echo "linux"
            ;;
        darwin)
            echo "darwin"
            ;;
        mingw*|msys*|cygwin*|windows*)
            echo "windows"
            ;;
        *)
            log_error "Unsupported operating system: $os"
            exit 1
            ;;
    esac
}

# Detect the CPU architecture
detect_arch() {
    arch="$(uname -m)"
    case "$arch" in
        x86_64|amd64)
            echo "amd64"
            ;;
        aarch64|arm64)
            echo "arm64"
            ;;
        *)
            log_error "Unsupported architecture: $arch"
            exit 1
            ;;
    esac
}

# Get the default installation directory following XDG spec
get_default_install_dir() {
    # XDG_BIN_HOME is not officially in the spec but is commonly used
    if [ -n "$XDG_BIN_HOME" ]; then
        echo "$XDG_BIN_HOME"
    elif [ -n "$HOME" ]; then
        echo "$HOME/.local/bin"
    else
        echo "/usr/local/bin"
    fi
}

# Get the latest version from GitHub releases
get_latest_version() {
    response=""
    if has_command curl; then
        response=$(curl -fsSL "https://api.github.com/repos/${GITHUB_OWNER}/${GITHUB_REPO}/releases/latest" 2>/dev/null)
    elif has_command wget; then
        response=$(wget -qO- "https://api.github.com/repos/${GITHUB_OWNER}/${GITHUB_REPO}/releases/latest" 2>/dev/null)
    else
        log_error "Neither curl nor wget found. Please install one of them."
        exit 1
    fi

    # Extract tag_name and strip optional 'v' prefix
    # Handles both "v1.0.0" and "1.0.0" tag formats
    version=$(echo "$response" | grep '"tag_name"' | sed -E 's/.*"tag_name"[[:space:]]*:[[:space:]]*"v?([^"]+)".*/\1/')

    if [ -z "$version" ]; then
        log_error "Failed to fetch latest version. Please specify BLOBBER_VERSION."
        exit 1
    fi

    echo "$version"
}

# Download a file
download() {
    url="$1"
    output="$2"

    log_info "Downloading $url"

    if has_command curl; then
        curl -fsSL "$url" -o "$output"
    elif has_command wget; then
        wget -q "$url" -O "$output"
    else
        log_error "Neither curl nor wget found. Please install one of them."
        exit 1
    fi
}

# Verify checksum
verify_checksum() {
    archive="$1"
    checksums_file="$2"
    archive_name="$3"

    log_info "Verifying checksum..."

    # Use awk for exact filename matching to avoid substring matches
    expected_checksum=$(awk -v name="$archive_name" '$2 == name { print $1 }' "$checksums_file")
    if [ -z "$expected_checksum" ]; then
        log_error "Could not find checksum for $archive_name"
        return 1
    fi

    if has_command sha256sum; then
        actual_checksum=$(sha256sum "$archive" | awk '{print $1}')
    elif has_command shasum; then
        actual_checksum=$(shasum -a 256 "$archive" | awk '{print $1}')
    elif has_command openssl; then
        actual_checksum=$(openssl dgst -sha256 "$archive" | awk '{print $NF}')
    else
        log_error "No SHA256 tool found (sha256sum, shasum, or openssl required)."
        log_error "Cannot verify archive integrity."
        return 1
    fi

    if [ "$expected_checksum" != "$actual_checksum" ]; then
        log_error "Checksum verification failed!"
        log_error "Expected: $expected_checksum"
        log_error "Actual:   $actual_checksum"
        return 1
    fi

    log_success "Checksum verified"
    return 0
}

# Verify signature using cosign
verify_signature() {
    checksums_file="$1"
    signature_file="$2"
    certificate_file="$3"

    if [ "${BLOBBER_NO_VERIFY:-0}" = "1" ]; then
        log_warn "Signature verification disabled via BLOBBER_NO_VERIFY"
        return 0
    fi

    if ! has_command cosign; then
        log_warn "cosign not found. Skipping signature verification."
        log_warn "Install cosign for enhanced security: https://docs.sigstore.dev/cosign/installation/"
        return 0
    fi

    log_info "Verifying signature..."

    if cosign verify-blob \
        --certificate "$certificate_file" \
        --signature "$signature_file" \
        --certificate-identity-regexp="$COSIGN_CERT_IDENTITY_REGEXP" \
        --certificate-oidc-issuer="$COSIGN_OIDC_ISSUER" \
        "$checksums_file" >/dev/null 2>&1; then
        log_success "Signature verified"
        return 0
    else
        log_error "Signature verification failed!"
        log_error "The checksums file may have been tampered with."
        return 1
    fi
}

# Extract only the binary from the archive
extract_binary() {
    archive="$1"
    dest="$2"
    binary="$3"
    os="$4"

    log_info "Extracting ${binary}..."

    if [ "$os" = "windows" ]; then
        if has_command unzip; then
            # Extract only the specific binary, flatten to dest directory
            unzip -q -j "$archive" "$binary" -d "$dest"
        else
            log_error "unzip not found. Please install it to extract .zip files."
            exit 1
        fi
    else
        if has_command tar; then
            # Extract only the specific binary
            # --strip-components=0 ensures we get just the file
            tar -xzf "$archive" -C "$dest" --strip-components=0 "$binary" 2>/dev/null \
                || tar -xzf "$archive" -C "$dest" "$binary"
        else
            log_error "tar not found. Please install it to extract .tar.gz files."
            exit 1
        fi
    fi

    # Verify the binary was extracted
    if [ ! -f "${dest}/${binary}" ]; then
        log_error "Failed to extract ${binary} from archive."
        exit 1
    fi
}

# Main installation function
main() {
    log_info "${BOLD}Blobber Installer${NC}"
    echo ""

    # Detect platform
    os=$(detect_os)
    arch=$(detect_arch)
    log_info "Detected platform: ${os}/${arch}"

    # Check for unsupported combinations
    if [ "$os" = "windows" ] && [ "$arch" = "arm64" ]; then
        log_error "Windows ARM64 is not supported"
        exit 1
    fi

    # Determine version
    version="${BLOBBER_VERSION:-}"
    if [ -z "$version" ]; then
        log_info "Fetching latest version..."
        version=$(get_latest_version)
    fi
    # Strip 'v' prefix if present
    version="${version#v}"
    log_info "Version: ${version}"

    # Determine installation directory
    install_dir="${BLOBBER_INSTALL:-$(get_default_install_dir)}"
    log_info "Installation directory: ${install_dir}"

    # Create temp directory
    # If BLOBBER_TMPDIR is set, create a subdirectory within it to avoid
    # accidentally deleting user data on cleanup
    if [ -n "$BLOBBER_TMPDIR" ]; then
        tmpdir=$(mktemp -d "${BLOBBER_TMPDIR}/blobber-install.XXXXXX")
    else
        tmpdir=$(mktemp -d)
    fi
    trap 'rm -rf -- "$tmpdir"' EXIT
    log_info "Using temp directory: ${tmpdir}"

    # Construct download URLs
    base_url="https://github.com/${GITHUB_OWNER}/${GITHUB_REPO}/releases/download/v${version}"

    if [ "$os" = "windows" ]; then
        archive_ext="zip"
    else
        archive_ext="tar.gz"
    fi

    archive_name="${BINARY_NAME}_${version}_${os}_${arch}.${archive_ext}"
    archive_url="${base_url}/${archive_name}"
    checksums_url="${base_url}/checksums.txt"
    signature_url="${base_url}/checksums.txt.sig"
    certificate_url="${base_url}/checksums.txt.pem"

    # Download files
    archive_path="${tmpdir}/${archive_name}"
    checksums_path="${tmpdir}/checksums.txt"
    signature_path="${tmpdir}/checksums.txt.sig"
    certificate_path="${tmpdir}/checksums.txt.pem"

    download "$archive_url" "$archive_path"
    download "$checksums_url" "$checksums_path"
    download "$signature_url" "$signature_path"
    download "$certificate_url" "$certificate_path"

    echo ""

    # Verify signature (of checksums file)
    if ! verify_signature "$checksums_path" "$signature_path" "$certificate_path"; then
        exit 1
    fi

    # Verify checksum (of archive)
    if ! verify_checksum "$archive_path" "$checksums_path" "$archive_name"; then
        exit 1
    fi

    echo ""

    # Determine binary name with extension
    binary_ext=""
    if [ "$os" = "windows" ]; then
        binary_ext=".exe"
    fi
    binary_file="${BINARY_NAME}${binary_ext}"

    # Extract only the binary from the archive
    extract_dir="${tmpdir}/extract"
    mkdir -p -- "$extract_dir"
    extract_binary "$archive_path" "$extract_dir" "$binary_file" "$os"

    # Create installation directory if it doesn't exist
    if [ ! -d "$install_dir" ]; then
        log_info "Creating installation directory: ${install_dir}"
        mkdir -p -- "$install_dir"
    fi

    # Install binary
    src_binary="${extract_dir}/${binary_file}"
    dest_binary="${install_dir}/${binary_file}"

    if [ -f "$dest_binary" ]; then
        log_info "Removing existing installation..."
        rm -f -- "$dest_binary"
    fi

    log_info "Installing ${BINARY_NAME} to ${dest_binary}"
    cp -- "$src_binary" "$dest_binary"
    chmod +x -- "$dest_binary"

    echo ""
    log_success "${BOLD}Successfully installed ${BINARY_NAME} v${version}${NC}"

    # Check if install directory is in PATH
    case ":$PATH:" in
        *":${install_dir}:"*)
            # Directory is in PATH
            ;;
        *)
            echo ""
            log_warn "Installation directory is not in your PATH."
            echo ""
            echo "Add the following to your shell profile (~/.bashrc, ~/.zshrc, etc.):"
            echo ""
            printf "    ${BOLD}export PATH=\"%s:\$PATH\"${NC}\n" "$install_dir"
            echo ""
            ;;
    esac

    # Verify installation
    if [ -x "$dest_binary" ]; then
        echo ""
        log_info "Verifying installation..."
        if "$dest_binary" version >/dev/null 2>&1; then
            echo ""
            "$dest_binary" version
        fi
    fi
}

# Run main function
main "$@"
