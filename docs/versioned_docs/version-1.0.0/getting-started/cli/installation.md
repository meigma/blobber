---
sidebar_position: 1
---

import Tabs from '@theme/Tabs';
import TabItem from '@theme/TabItem';

# Installation

Install the blobber CLI.

<Tabs groupId="install-method">
<TabItem value="script" label="Installer Script" default>

Download and install the latest release:

```bash
curl -fsSL https://blobber.meigma.dev/install.sh | sh
```

The script automatically:
- Detects your OS and architecture
- Verifies checksums (SHA256)
- Verifies signatures (if [cosign](https://docs.sigstore.dev/cosign/installation/) is installed)
- Installs to `~/.local/bin` (following XDG conventions)

### Options

Customize the installation with environment variables:

| Variable | Description | Default |
|----------|-------------|---------|
| `BLOBBER_VERSION` | Version to install | latest |
| `BLOBBER_INSTALL` | Installation directory | `~/.local/bin` |

**Install a specific version:**

```bash
curl -fsSL https://blobber.meigma.dev/install.sh | BLOBBER_VERSION=1.0.0 sh
```

**Install to a custom directory:**

```bash
curl -fsSL https://blobber.meigma.dev/install.sh | BLOBBER_INSTALL=/usr/local/bin sh
```

:::tip
Ensure `~/.local/bin` is in your `PATH`. Add this to your shell profile:

```bash
export PATH="$HOME/.local/bin:$PATH"
```
:::

</TabItem>
<TabItem value="homebrew" label="Homebrew">

Install via [Homebrew](https://brew.sh/) (macOS and Linux):

```bash
brew install meigma/tap/blobber
```

Upgrade to the latest version:

```bash
brew upgrade blobber
```

</TabItem>
<TabItem value="scoop" label="Scoop">

Install via [Scoop](https://scoop.sh/) (Windows):

```powershell
scoop bucket add meigma https://github.com/meigma/scoop-bucket
scoop install blobber
```

Upgrade to the latest version:

```powershell
scoop update blobber
```

</TabItem>
<TabItem value="nix" label="Nix">

Blobber provides a [Nix flake](https://nixos.wiki/wiki/Flakes) for installation and development.

### Run without installing

```bash
nix run github:meigma/blobber -- --help
```

### Install to profile

```bash
nix profile install github:meigma/blobber
```

### Add to your flake

```nix title="flake.nix"
{
  inputs = {
    blobber.url = "github:meigma/blobber";
  };

  outputs = { self, blobber, ... }: {
    # Use blobber.packages.${system}.default
  };
}
```

### Development shell

Clone the repository and enter the development environment:

```bash
git clone https://github.com/meigma/blobber.git
cd blobber
nix develop
```

</TabItem>
<TabItem value="go" label="Go">

If you have Go 1.21+ installed:

```bash
go install github.com/meigma/blobber/cmd/blobber@latest
```

:::note
This installs to `$GOPATH/bin` (usually `~/go/bin`). Ensure it's in your `PATH`.
:::

</TabItem>
<TabItem value="source" label="From Source">

Clone and build from source:

```bash
git clone https://github.com/meigma/blobber.git
cd blobber
go build -o blobber ./cmd/blobber
```

Move the binary to your PATH:

```bash
mv blobber ~/.local/bin/
# or
sudo mv blobber /usr/local/bin/
```

</TabItem>
</Tabs>

## Verify Installation

```bash
blobber --help
```

## Requirements

- **Docker credentials** configured for authenticated registries

Blobber uses your existing Docker credentials from `~/.docker/config.json`. If you can `docker push` to a registry, blobber can too.

## Next Steps

- [Quickstart](./quickstart.md) - Push and pull your first files
- [CLI Tutorial](../../tutorials/cli-basics.md) - Learn all CLI features step-by-step
