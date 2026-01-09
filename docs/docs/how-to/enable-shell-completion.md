---
sidebar_position: 10
---

# How to Enable Shell Completion

This guide shows how to enable shell auto-completion for blobber commands, flags, image tags, and file paths.

## What You Get

Once enabled, pressing `Tab` will complete:

- **Commands and subcommands**: `blobber pu<TAB>` → `blobber push`
- **Flags**: `blobber push --com<TAB>` → `blobber push --compression`
- **Image tags**: `ghcr.io/org/repo:<TAB>` → suggests available tags from the registry
- **File paths in images**: `blobber cat ghcr.io/org/repo:v1 conf<TAB>` → suggests matching files

## Bash

### Prerequisites

Install the `bash-completion` package:

```bash
# macOS
brew install bash-completion

# Debian/Ubuntu
sudo apt install bash-completion

# Fedora/RHEL
sudo dnf install bash-completion
```

### Enable Completion

**For the current session:**

```bash
source <(blobber completion bash)
```

**For all future sessions:**

```bash
# macOS (Homebrew)
blobber completion bash > $(brew --prefix)/etc/bash_completion.d/blobber

# Linux
blobber completion bash > /etc/bash_completion.d/blobber
```

Restart your shell or run `source ~/.bashrc`.

## Zsh

### Prerequisites

Ensure completion is enabled in your `~/.zshrc`:

```bash
echo "autoload -U compinit; compinit" >> ~/.zshrc
```

### Enable Completion

**For the current session:**

```bash
source <(blobber completion zsh)
```

**For all future sessions:**

```bash
# macOS (Homebrew)
blobber completion zsh > $(brew --prefix)/share/zsh/site-functions/_blobber

# Linux
blobber completion zsh > "${fpath[1]}/_blobber"
```

Restart your shell.

## Fish

**For the current session:**

```fish
blobber completion fish | source
```

**For all future sessions:**

```fish
blobber completion fish > ~/.config/fish/completions/blobber.fish
```

Restart your shell.

## PowerShell

**For the current session:**

```powershell
blobber completion powershell | Out-String | Invoke-Expression
```

**For all future sessions:**

Add the above command to your PowerShell profile. To find your profile path:

```powershell
echo $PROFILE
```

## Disabling Descriptions

By default, completions include helpful descriptions. To disable them:

```bash
blobber completion bash --no-descriptions > /path/to/completion
```

## Troubleshooting

### Completions not appearing

1. Ensure you restarted your shell after setup
2. Verify the completion file exists in the correct location
3. For bash, ensure `bash-completion` is installed and sourced

### Tag completion is slow

Tag lists are fetched from the registry on first use. They're cached for 30 seconds to speed up subsequent completions. If your registry is slow, completions may take a moment on first access.

### Tag completion requires authentication

If your registry requires authentication, ensure you're logged in:

```bash
docker login ghcr.io
```

See [How to Authenticate](/docs/how-to/authenticate) for details.

## See Also

- [blobber completion](/docs/reference/cli/completion) - Command reference
- [How to Authenticate](/docs/how-to/authenticate) - Registry authentication
