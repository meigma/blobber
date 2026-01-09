---
sidebar_position: 8
---

# blobber completion

Generate shell auto-completion scripts.

## Synopsis

```bash
blobber completion <shell> [flags]
```

## Description

Generates auto-completion scripts for blobber commands, flags, and arguments. The generated script enables tab completion in your shell.

Supported shells:

- `bash` - Bash shell
- `zsh` - Z shell
- `fish` - Fish shell
- `powershell` - PowerShell

## Subcommands

| Subcommand | Description |
|------------|-------------|
| `bash` | Generate completion script for Bash |
| `zsh` | Generate completion script for Zsh |
| `fish` | Generate completion script for Fish |
| `powershell` | Generate completion script for PowerShell |

## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--no-descriptions` | bool | `false` | Disable completion descriptions |

## Output

Outputs the completion script to stdout. Redirect to a file for permanent installation.

## Dynamic Completions

The generated script enables dynamic completions that query registries at runtime:

| Context | Completion |
|---------|------------|
| Image reference with `:` | Available tags from the registry |
| File path argument | Files within the referenced image |
| Directories | Local filesystem directories |

Tag lists are cached for 30 seconds to improve performance.

## Examples

Generate and source immediately (Bash):

```bash
source <(blobber completion bash)
```

Generate and save to file (Zsh):

```bash
blobber completion zsh > ~/.zsh/completions/_blobber
```

Generate without descriptions:

```bash
blobber completion bash --no-descriptions
```

## Exit Codes

| Code | Description |
|------|-------------|
| 0 | Success |
| 1 | Invalid shell name |

## See Also

- [How to Enable Shell Completion](/docs/how-to/enable-shell-completion) - Setup guide
