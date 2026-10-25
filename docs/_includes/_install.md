<!-- markdownlint-disable MD033 MD041 MD024 MD022 -->

# Installation
{: .no_toc }

## xget
{: .no_toc }

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
