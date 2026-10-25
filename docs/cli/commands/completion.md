---
layout: default
title: 🖥️ Completions
nav_order: 1
parent: 🖥️ Commands
grand_parent: 🖥️ CLI
---

<!-- markdownlint-disable MD025 MD022 -->
# 🖥️ Completions
{: .no_toc }

## Usage

```bash
ironstate completion <SHELL>
```

## Available Shells

> [!NOTE]
> Be sure to restart or reload your shell after adding the completion commands.

### Bash

To enable Bash completions, add the following to your `~/.bashrc`:

```bash
source <(ironstate completion bash)
```

### Zsh

To enable Zsh completions, add the following to your `~/.zshrc`:

```bash
source <(ironstate completion zsh)
```

### Fish

To enable Fish completions, add the following to your `~/.config/fish/config.fish`:

```bash
ironstate completion fish | source
```

### PowerShell (pwsh)

To enable PowerShell completions, add the following to your PowerShell profile (e.g., `~\.config\powershell\Microsoft.PowerShell_profile.ps1`):

```powershell
ironstate completion powershell | Out-String | Invoke-Expression
```

> [!NOTE]
> The shell argument is `powershell` (not `pwsh`); the generated script works in both Windows PowerShell and PowerShell 7+.

## Flags

| Flag | Description |
| --- | --- |
| `--no-descriptions` | Generate completions without command/flag descriptions. |
| `-h`, `--help` | Show help for the completion command. |
