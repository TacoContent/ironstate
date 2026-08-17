#!/usr/bin/env pwsh
<#
.SYNOPSIS
  Loading, merging, and modular-expansion logic for the ironstate.ps1 runner.
#>

Set-StrictMode -Version Latest

Import-Module (Join-Path $PSScriptRoot 'Common.psm1')
Import-Module (Join-Path $PSScriptRoot 'Templates.psm1')

function Import-EnvFile {
  # Loads KEY=VALUE lines from a dotenv-style file into the current
  # process's environment, mirroring apply.sh's `source .env`/`.secrets`.
  param([Parameter(Mandatory)][string] $Path)

  if (-not (Test-Path $Path)) { return }
  foreach ($line in Get-Content -Path $Path) {
    $trimmed = $line.Trim()
    if (-not $trimmed -or $trimmed.StartsWith('#')) { continue }

    $idx = $trimmed.IndexOf('=')
    if ($idx -lt 1) { continue }

    $key = $trimmed.Substring(0, $idx).Trim()
    $value = $trimmed.Substring($idx + 1).Trim()
    if (($value.StartsWith('"') -and $value.EndsWith('"')) -or ($value.StartsWith("'") -and $value.EndsWith("'"))) {
      $value = $value.Substring(1, $value.Length - 2)
    }
    Set-Item -Path "Env:$key" -Value $value
  }
}

function Initialize-YamlModule {
  if (-not (Get-Module -ListAvailable -Name powershell-yaml)) {
    Write-Verbose "powershell-yaml module not found; installing for current user..."
    Install-Module -Name powershell-yaml -Scope CurrentUser -Force
  }
  Import-Module powershell-yaml
}

function Import-PackagesFile {
  # Loads a single YAML file and resolves its 'copy.src' / 'shell.script'
  # paths relative to $BaseDir (defaults to the file's own directory).
  param(
    [Parameter(Mandatory)][string] $Path,
    [string] $BaseDir
  )

  if (-not (Test-Path $Path)) { throw "Packages file not found: $Path" }
  if (-not $BaseDir) { $BaseDir = Split-Path $Path -Parent }

  $yaml = Get-Content -Raw -Path $Path
  $data = ConvertFrom-Yaml -Yaml $yaml -Ordered
  # An empty (or comments/whitespace-only) document parses to $null rather
  # than an empty mapping/list - treat it as an empty task list, same as
  # Tasks.psm1's Get-TaskList already treats '{ tasks: [] }' and '[]' the
  # same way, rather than failing downstream on a null 'Data' bind.
  if ($null -eq $data) { $data = [ordered]@{} }

  Resolve-RelativePathsInPlace -Data $data -BaseDir $BaseDir
  return $data
}

function Merge-VarsData {
  # Unlike list groups (appended) or plain scalars (replaced), 'vars:' is a
  # mapping that deep-merges key-by-key, so a host/user overlay can
  # add/override individual vars without wholesale-replacing the base set.
  param([Parameter(Mandatory)] $Base, [Parameter(Mandatory)] $Overlay)

  $result = [ordered]@{}
  foreach ($key in $Base.Keys) { $result[$key] = $Base[$key] }

  foreach ($key in $Overlay.Keys) {
    if ($result.Contains($key) -and
        ($result[$key] -is [System.Collections.IDictionary]) -and
        ($Overlay[$key] -is [System.Collections.IDictionary])) {
      $result[$key] = Merge-VarsData -Base $result[$key] -Overlay $Overlay[$key]
    } else {
      $result[$key] = $Overlay[$key]
    }
  }
  return $result
}

function Merge-PackagesData {
  param(
    [Parameter(Mandatory)] $Base,
    [Parameter(Mandatory)] $Overlay
  )

  $result = [ordered]@{}
  foreach ($key in $Base.Keys) { $result[$key] = $Base[$key] }

  foreach ($key in $Overlay.Keys) {
    if ($key -eq 'vars' -and $result.Contains('vars') -and
        ($result['vars'] -is [System.Collections.IDictionary]) -and
        ($Overlay['vars'] -is [System.Collections.IDictionary])) {
      $result['vars'] = Merge-VarsData -Base $result['vars'] -Overlay $Overlay['vars']
    } elseif ($result.Contains($key) -and
        ($result[$key] -is [System.Collections.IList]) -and
        ($Overlay[$key] -is [System.Collections.IList])) {
      $merged = [System.Collections.Generic.List[object]]::new()
      foreach ($i in $result[$key]) { $merged.Add($i) }
      foreach ($i in $Overlay[$key]) { $merged.Add($i) }
      $result[$key] = $merged
    } else {
      $result[$key] = $Overlay[$key]
    }
  }
  return $result
}

function Import-PackagesHierarchy {
  # Loads base file then merges host- and user-specific overlays (Ansible-style).
  # All three files resolve 'copy.src' / 'shell.script' relative to the same
  # install/windows root, regardless of which subdirectory the overlay lives in.
  param([Parameter(Mandatory)][string] $BaseFile)

  $dir  = Split-Path $BaseFile -Parent
  $data = Import-PackagesFile -Path $BaseFile -BaseDir $dir

  $hostFile = Join-Path $dir "hosts\$($env:COMPUTERNAME).yml"
  $userFile = Join-Path $dir "variables\$($env:USERNAME).yml"

  foreach ($file in @($hostFile, $userFile)) {
    if (Test-Path $file) {
      Write-Verbose "Merging overlay: $file"
      $overlay = Import-PackagesFile -Path $file -BaseDir $dir
      $data    = Merge-PackagesData -Base $data -Overlay $overlay
    }
  }
  return $data
}

function Import-IncludedPackage {
  # Loads packages/<name>/main.yml for the 'include' module (Tasks.psm1) and
  # applies its template context - the include-equivalent of an 'actions:'
  # list, just sourced from an external file instead of written inline.
  #
  # Nothing on the include spec is applied to the package automatically:
  # 'state'/'tags'/'with' are only made available inside the package as
  # '${{ package.state }}'/'${{ package.tags }}'/'${{ inputs.<key> }}'
  # template expressions (see Templates.psm1); a package author opts in by
  # writing the expression on whichever field(s) should receive it. Returns
  # the raw parsed document ($null on failure) - its root shape (explicit
  # 'tasks:' or bare list) is resolved by the caller via Tasks.psm1's
  # Get-TaskList, same as any other loaded document.
  #
  # A package's own top-level 'vars:' block (if any) declares package-local
  # defaults - bare top-level names distinct from site-level '${{ vars.* }}'
  # (e.g. 'vars: { default: { jdk: ... } }' -> '${{ default.jdk }}'), so a
  # package can express "site override, else my own built-in default" via
  # '${{ languages.java.jdk | default(default.languages.java.jdk) }}'. This
  # only wires them into *this* pass's context (site-only bare vars like
  # 'languages.java.jdk' above still resolve at ironstate.ps1's later strict
  # pass, via Tasks.psm1's per-leaf 'PackageVars') - a field referencing
  # only its own local var can resolve here already, though.
  param(
    [Parameter(Mandatory)] $IncludeSpec,
    [Parameter(Mandatory)][string] $PackagesRoot,
    # Untyped (not [hashtable]): YAML-parsed values come back as [ordered]
    # OrderedDictionary, which doesn't bind to a [hashtable] param.
    $Facts = @{},
    $Vars = @{}
  )

  $name = Get-Prop $IncludeSpec 'name'
  if (-not $name) { Write-Warning "include has no 'name'; skipping"; return $null }

  $pkgDir  = Join-Path $PackagesRoot $name
  $pkgFile = Join-Path $pkgDir 'main.yml'
  if (-not (Test-Path $pkgFile)) { Write-Warning "Included package '$name' not found: $pkgFile"; return $null }

  Write-Verbose "Loading included package '$name' from $pkgFile"
  $pkgData = Import-PackagesFile -Path $pkgFile -BaseDir $pkgDir

  $context = @{
    package = @{
      name  = $name
      state = Get-Prop $IncludeSpec 'state' 'present'
      tags  = @(Get-Prop $IncludeSpec 'tags' @())
    }
    inputs = Get-Prop $IncludeSpec 'with' @{}
    facts  = $Facts
    vars   = $Vars
  }
  $reservedNamespaces = @('package', 'inputs', 'facts', 'vars')
  foreach ($key in (Get-Prop $pkgData 'vars' @{}).Keys) {
    if ($reservedNamespaces -contains $key) {
      Write-Warning "Package '$name': its own 'vars.$key' collides with a reserved namespace name; ignoring."
      continue
    }
    $context[$key] = $pkgData['vars'][$key]
  }
  # '-Soft': a bare id/fact reference (e.g. '${{ bar }}') doesn't belong to
  # any of {package, inputs, facts, vars, <package's own vars keys>} and
  # can't be resolved yet - leave it untouched for ironstate.ps1's per-leaf
  # dispatch-time pass, once the registry that reference points at actually
  # exists.
  Resolve-TemplatesInPlace -Data $pkgData -Context $context -PackageName $name -Soft

  return $pkgData
}

Export-ModuleMember -Function Import-EnvFile, Initialize-YamlModule, Import-PackagesFile, Merge-PackagesData, Import-PackagesHierarchy, Import-IncludedPackage
