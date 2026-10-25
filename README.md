<!-- markdownlint-disable MD033 MD024 -->
# ironstate

`ironstate` is a declarative, Ansible-style task runner driven by YAML. You describe the state a machine should be in, preview the changes, and then apply them, all from a single cross-platform binary for Windows, Linux, and macOS.

A playbook is a directory of YAML task files. ironstate checks each task's desired `state` (`present`, `absent`, `latest`) against what's already on the machine. Every run is a **dry-run** by default: ironstate prints what it *would* change, and nothing is modified until you pass `--apply`.

📚 [Full documentation](https://tacocontent.github.io/ironstate/)

## Highlights

- **One binary, every desktop OS.** Windows, Linux (amd64, arm64, armv7), and macOS (Intel and Apple Silicon).
- **Dry-run first.** Every run predicts what would change. `--apply` is always explicit.
- **Batteries included.** Built-in handlers for winget, Chocolatey, Scoop, Homebrew, apt, pacman, yum, apk, snap, flatpak, pipx, npm, cargo, go, git, files, templates, shell commands, firewall rules, users, cron, the Windows registry, and more.
- **Host-aware playbooks.** `hosts/` and `variables/` overlays and task files are picked automatically from the machine's hostname, OS family, distribution, platform, and architecture.
- **Expressions and filters.** `when` conditions and `${{ }}` templates share one expression language with a filter pipeline you can extend with your own scripts.
- **Reusable packages.** Split work into roles and packages with `include`, or share them across repositories with `uses`.
- **Remote apply.** Run the same playbook on other machines over SSH with `--target` or an inventory file.
- **Extensible.** Write external handler plugins in Go with the ironstate SDK.

## Installation

## xget

[xget](https://github.com/camalot/xget) is the simplest, cross-platform way to install Ironstate.

```bash
xget install TacoContent/ironstate --to="~/.local/bin" --ignore=".sbom.json"
```

---

<details markdown="block">
<summary markdown="span"><b>Windows</b></summary>

## powershell

> [!NOTE]
> When run from a downloaded file, the quick-install scripts will download and verify the sha256 checksum of the install script before continuing. When piped directly to `bash` or `iex`, script checksum verification is skipped because there is no local script file to hash. Release asset checksums are still verified.

```powershell
iwr https://raw.githubusercontent.com/TacoContent/ironstate/refs/heads/develop/install/install.ps1 | iex
```

## bash

```shell
curl -fsSL https://raw.githubusercontent.com/TacoContent/ironstate/refs/heads/develop/install/install.sh| bash
```

> [!NOTE]
> The default install location is `$HOME/.local/bin`. You can change the install location with the `-d` or `--dir` option

``` shell
curl -fsSL -o install.sh https://raw.githubusercontent.com/TacoContent/ironstate/refs/heads/develop/install/install.sh && bash install.sh --dir /usr/local/bin
```

## scoop

```shell
scoop bucket add camalot https://github.com/camalot/scoop
scoop install camalot/ironstate
```

<!-- NOT YET AVAILABLE 
## WinGet

```shell
winget install camalot.ironstate
```

## Chocolatey

```shell
choco install ironstate
```
-->

</details>

---

<details markdown="block">
<summary markdown="span"><b>Linux</b></summary>

## bash

```shell
curl -fsSL https://raw.githubusercontent.com/TacoContent/ironstate/refs/heads/develop/install/install.sh | bash
```

> [!NOTE]
> The default install location is `$HOME/.local/bin`. You can change the install location with the `-d` or `--dir` option

``` shell
curl -fsSL -o install.sh https://raw.githubusercontent.com/TacoContent/ironstate/refs/heads/develop/install/install.sh && bash install.sh --dir /usr/local/bin
```

## powershell

```powershell
iwr https://raw.githubusercontent.com/TacoContent/ironstate/refs/heads/develop/install/install.ps1 | iex
```

## homebrew

```shell
brew install camalot/scoop/ironstate
```

</details>

---

<details markdown="block">
<summary markdown="span"><b>macOS</b></summary>

## bash

```shell
curl -fsSL https://raw.githubusercontent.com/TacoContent/ironstate/refs/heads/develop/install/install.sh | bash
```

> [!NOTE]
> The default install location is `$HOME/.local/bin`. You can change the install location with the `-d` or `--dir` option

``` shell
curl -fsSL -o install.sh https://raw.githubusercontent.com/TacoContent/ironstate/refs/heads/develop/install/install.sh && bash install.sh --dir /usr/local/bin
```

## powershell

```powershell
iwr https://raw.githubusercontent.com/TacoContent/ironstate/refs/heads/develop/install/install.ps1 | iex
```

## homebrew

```shell
brew install camalot/scoop/ironstate
```

</details>

---

<details markdown="block">
<summary markdown="span"><b>Android</b></summary>

## bash

```shell
curl -fsSL https://raw.githubusercontent.com/TacoContent/ironstate/refs/heads/develop/install/install.sh | bash
```

> [!NOTE]
> The default install location is `$HOME/.local/bin`. You can change the install location with the `-d` or `--dir` option

``` shell
curl -fsSL -o install.sh https://raw.githubusercontent.com/TacoContent/ironstate/refs/heads/develop/install/install.sh && bash install.sh --dir /usr/local/bin
```

</details>

---

<details markdown="block">
<summary markdown="span"><b>Checksum Verification</b></summary>

```shell
curl -fsSLO https://github.com/TacoContent/ironstate/releases/download/v<VERSION>/checksums.txt
sha256sum --check --ignore-missing checksums.txt
```

</details>

## Quick start

```shell
# Scaffold a new playbook
ironstate init my-playbook

# Preview what would change (dry-run)
ironstate --playbook my-playbook

# Apply the changes
ironstate --playbook my-playbook --apply

# Only run tasks with certain tags
ironstate --playbook my-playbook --tags cli,security --apply
```

A minimal `main.yml`:

```yaml
tasks:
  - name: Install ripgrep
    tags: [cli]
    winget:
      package: BurntSushi.ripgrep.MSVC
    when: platform == "windows"

  - name: Install ripgrep
    tags: [cli]
    become: true
    apt:
      package: ripgrep
    when: os_family == "debian"
```

See [Getting Started](https://tacocontent.github.io/ironstate/gettingstarted.html) for a full walkthrough.

## Documentation

The full documentation lives in [`docs/`](docs/) and is published at <https://tacocontent.github.io/ironstate/>.

- [Home](https://tacocontent.github.io/ironstate/)
- [Installation](https://tacocontent.github.io/ironstate/installation.html)
- [Getting Started](https://tacocontent.github.io/ironstate/gettingstarted.html)
- [Playbooks](https://tacocontent.github.io/ironstate/playbooks/)
  - [Folder Structure](https://tacocontent.github.io/ironstate/playbooks/structure.html)
  - [Task Files](https://tacocontent.github.io/ironstate/playbooks/task-files.html)
  - [Tasks](https://tacocontent.github.io/ironstate/playbooks/tasks.html)
  - [File Naming](https://tacocontent.github.io/ironstate/playbooks/naming.html)
  - [Hosts and Variables](https://tacocontent.github.io/ironstate/playbooks/hosts.html)
  - [Facts and Variables](https://tacocontent.github.io/ironstate/playbooks/facts-vars.html)
  - [Conditions and Expressions](https://tacocontent.github.io/ironstate/playbooks/conditions.html)
  - [Loops](https://tacocontent.github.io/ironstate/playbooks/loops.html)
  - [Results and Failures](https://tacocontent.github.io/ironstate/playbooks/results.html)
  - [Tags](https://tacocontent.github.io/ironstate/playbooks/tags.html)
  - [Environment Variables](https://tacocontent.github.io/ironstate/playbooks/environment.html)
  - [Includes](https://tacocontent.github.io/ironstate/playbooks/includes.html)
  - [Remote Sources](https://tacocontent.github.io/ironstate/playbooks/uses.html)
  - [Secrets](https://tacocontent.github.io/ironstate/playbooks/secrets.html)
  - [Inventory](https://tacocontent.github.io/ironstate/playbooks/inventory.html)
- [Handlers](https://tacocontent.github.io/ironstate/handlers/)
- [Filters](https://tacocontent.github.io/ironstate/filters/)
  - [Built-in Filters](https://tacocontent.github.io/ironstate/filters/builtin.html)
  - [Custom Filters](https://tacocontent.github.io/ironstate/filters/custom.html)
- [CLI](https://tacocontent.github.io/ironstate/cli/)
  - [Commands](https://tacocontent.github.io/ironstate/cli/commands/)
  - [Flags](https://tacocontent.github.io/ironstate/cli/flags.html)
- [Plugins](https://tacocontent.github.io/ironstate/plugins/)
  - [Create A Plugin](https://tacocontent.github.io/ironstate/plugins/create.html)
  - [Using A Plugin](https://tacocontent.github.io/ironstate/plugins/using.html)
  - [Handler Contract](https://tacocontent.github.io/ironstate/plugins/contract.html)
  - [Plugin Testing](https://tacocontent.github.io/ironstate/plugins/test.html)
  - [Building A Plugin](https://tacocontent.github.io/ironstate/plugins/build.html)
- [SDK](https://tacocontent.github.io/ironstate/sdk/)
- [Changelog](https://tacocontent.github.io/ironstate/changelog.html)
- [Support](https://tacocontent.github.io/ironstate/support.html)
- [Contributing](https://tacocontent.github.io/ironstate/contributing.html)
- [Code of Conduct](https://tacocontent.github.io/ironstate/code-of-conduct.html)
- [License](https://tacocontent.github.io/ironstate/license.html)

## Contributing

Contributions are welcome. See [CONTRIBUTING.md](CONTRIBUTING.md) and the [Code of Conduct](CODE_OF_CONDUCT.md). For help, see [SUPPORT.md](SUPPORT.md).

## License

See [LICENSE](LICENSE).
