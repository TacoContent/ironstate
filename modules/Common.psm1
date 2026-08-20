#!/usr/bin/env pwsh
<#
.SYNOPSIS
  Shared helpers used by ironstate.ps1 and every handler module.
#>

Set-StrictMode -Version Latest

function Get-Prop {
  # Safe dictionary read: yaml items are shaped differently per group
  # (e.g. symlinks have no 'package' key), and Set-StrictMode throws on
  # dot-access of a missing key, so every optional field goes through here.
  param($Item, [Parameter(Mandatory)][string] $Name, $Default = $null)
  if ($null -ne $Item -and $Item.Contains($Name)) { return $Item[$Name] }
  return $Default
}

function Resolve-UserPath {
  param([Parameter(Mandatory)][string] $Path)

  if ($Path -eq '~') { return $HOME }
  if ($Path.StartsWith('~/') -or $Path.StartsWith('~\')) {
    return Join-Path $HOME $Path.Substring(2)
  }
  return $Path
}

function Resolve-InstallRelativePath {
  # Resolves a 'copy.src' / 'shell.script' / 'template.src' path against the
  # directory that owns the YAML file it came from (install/windows root, or
  # a package's own directory for packages/<name>/main.yml). URLs, '~'
  # paths, and already-rooted paths pass through untouched.
  param([Parameter(Mandatory)][string] $Path, [Parameter(Mandatory)][string] $BaseDir)

  if ($Path -match '^(https?://|~)') { return $Path }
  if ([System.IO.Path]::IsPathRooted($Path)) { return $Path }
  return (Join-Path $BaseDir $Path)
}

function Resolve-RelativePathsInTaskList {
  # Recurses through a task/action list (same 'actions' grouping rule as
  # Tasks.psm1's Expand-TaskTree, but structural only - no when/tags) looking
  # for 'copy'/'shell'/'template'/'blockinfile.template' leaves at any depth.
  param($Tasks, [Parameter(Mandatory)][string] $BaseDir)

  foreach ($item in @($Tasks)) {
    if ($null -eq $item -or -not ($item -is [System.Collections.IDictionary])) { continue }

    if ($item.Contains('actions')) {
      Resolve-RelativePathsInTaskList -Tasks (Get-Prop $item 'actions' @()) -BaseDir $BaseDir
      continue
    }

    if ($item.Contains('copy')) {
      $src = Get-Prop $item['copy'] 'src'
      if ($src) { $item['copy']['src'] = Resolve-InstallRelativePath -Path $src -BaseDir $BaseDir }
    }

    if ($item.Contains('template')) {
      $src = Get-Prop $item['template'] 'src'
      if ($src) { $item['template']['src'] = Resolve-InstallRelativePath -Path $src -BaseDir $BaseDir }
    }

    if ($item.Contains('blockinfile')) {
      $tpl = Get-Prop $item['blockinfile'] 'template'
      if ($tpl -is [System.Collections.IDictionary]) {
        $tplSrc = Get-Prop $tpl 'src'
        if ($tplSrc) { $tpl['src'] = Resolve-InstallRelativePath -Path $tplSrc -BaseDir $BaseDir }
      }
    }

    if ($item.Contains('shell')) {
      $scriptPath = Get-Prop $item['shell'] 'script'
      if ($scriptPath) { $item['shell']['script'] = Resolve-InstallRelativePath -Path $scriptPath -BaseDir $BaseDir }

      # A per-state block ('present'/'absent'/'latest') can carry its own
      # 'script' too - see Handlers/Shell.psm1's Resolve-ShellStateConfig.
      foreach ($state in @('present', 'absent', 'latest')) {
        $block = Get-Prop $item['shell'] $state
        if ($block -isnot [System.Collections.IDictionary]) { continue }
        $blockScript = Get-Prop $block 'script'
        if ($blockScript) { $block['script'] = Resolve-InstallRelativePath -Path $blockScript -BaseDir $BaseDir }
      }
    }
  }
}

function Resolve-RelativePathsInPlace {
  # Rewrites 'copy.src' and 'shell.script' fields in-place from
  # install-relative to absolute, immediately after a YAML file is loaded.
  param([Parameter(Mandatory)] $Data, [Parameter(Mandatory)][string] $BaseDir)

  $taskList =
    if ($Data -is [System.Collections.IDictionary]) { if ($Data.Contains('tasks')) { @($Data['tasks']) } else { @() } }
    elseif ($Data -is [System.Collections.IList]) { @($Data) }
    else { @() }

  Resolve-RelativePathsInTaskList -Tasks $taskList -BaseDir $BaseDir
}

function Get-ItemLabel {
  param($Item)
  $package = Get-Prop $Item 'package'
  if ($package) { return $package }
  $name = Get-Prop $Item 'name'
  if ($name) { return $name }
  $dest = Get-Prop $Item 'dest'
  if ($dest) { return $dest }
  $path = Get-Prop $Item 'path'
  if ($path) { return $path }
  $scriptPath = Get-Prop $Item 'script'
  if ($scriptPath) { return $scriptPath }
  $command = Get-Prop $Item 'command'
  if ($command) { return (@($command -split "`n") | Select-Object -First 1).Trim() }
  return '<unknown>'
}

function Get-ItemState {
  param($Item)
  return Get-Prop $Item 'state' 'present'
}

# --- exec result capture (shared by external-CLI handlers) ---------------

function Get-ZeroExecResult {
  # The zero/default shape for a leaf's execution result - used whenever a
  # handler's Install/Uninstall doesn't return a real result (dry-run, a
  # Skip, or a pure-PowerShell handler that succeeded without reporting one).
  @{ rc = 0; stdout = ''; stdout_lines = @(); stderr = ''; stderr_lines = @() }
}

function Invoke-ExternalCommand {
  # Runs an external CLI (choco/winget/pipx/npm/cargo/go/eget/...), capturing
  # stdout/stderr to temp files and the exit code, then echoing what was
  # captured (Write-Host for stdout, Write-Warning for stderr) so behavior
  # still resembles live output - the same trade-off Shell.psm1 already
  # makes for its non-'pwsh' hosts (output arrives after the command
  # finishes, not streamed line-by-line). Returns
  # { rc, stdout, stdout_lines, stderr, stderr_lines }.
  param([Parameter(Mandatory)][string] $Exe, [string[]] $Arguments = @())

  $stdoutFile = [System.IO.Path]::Combine([System.IO.Path]::GetTempPath(), [System.IO.Path]::GetRandomFileName() + '.out')
  $stderrFile = [System.IO.Path]::Combine([System.IO.Path]::GetTempPath(), [System.IO.Path]::GetRandomFileName() + '.err')

  try {
    $global:LASTEXITCODE = $null
    & $Exe @Arguments 6>&1 1>$stdoutFile 2>$stderrFile
    $rc = Get-Variable -Name LASTEXITCODE -ValueOnly -ErrorAction SilentlyContinue
    if ($null -eq $rc) { $rc = 0 }
    $stdoutLines = @(if (Test-Path $stdoutFile) { @(Get-Content -Path $stdoutFile) } else { @() })
    $stderrLines = @(if (Test-Path $stderrFile) { @(Get-Content -Path $stderrFile) } else { @() })
  } finally {
    Remove-Item -Path $stdoutFile, $stderrFile -Force -ErrorAction SilentlyContinue
  }

  foreach ($line in $stdoutLines) { Write-Host $line }
  foreach ($line in $stderrLines) { Write-Warning $line }

  return @{
    rc           = $rc
    stdout       = ($stdoutLines -join "`n")
    stdout_lines = $stdoutLines
    stderr       = ($stderrLines -join "`n")
    stderr_lines = $stderrLines
  }
}

# --- state reconciliation (pure) -----------------------------------------

function Resolve-PackageAction {
  param([string] $State, [bool] $IsInstalled)

  switch ($State) {
    'present' { if ($IsInstalled) { return 'Skip' } else { return 'Install' } }
    'latest' { return 'Install' }
    'absent' { if ($IsInstalled) { return 'Uninstall' } else { return 'Skip' } }
    default { throw "Unknown state '$State'" }
  }
}

# --- tag filtering (pure) ------------------------------------------------

function Merge-FlatContext {
  # Builds the single flat dict 'when' conditions and the per-leaf deferred
  # '${{ }}' pass resolve names against. Gathered host facts and
  # user-registered 'fact' values are merged together (user facts win on
  # collision) and nested under one 'facts' key - not flattened to bare
  # top-level names - so both need the 'facts.' prefix, matching the
  # documented '${{ facts.<key> }}' / bare 'facts.<key>' grammar. Everything
  # else - a leaf's owning package's own local vars (see Tasks.psm1's
  # 'PackageVars' on a leaf), then site-level vars (override), then the
  # growing id-registry (override) - stays flattened to bare top-level names:
  # last write wins, so a site-level var can shadow a package's own local
  # default of the same name (Ansible role-defaults-style precedence).
  param($Facts = @{}, $UserFacts = @{}, $PackageVars = @{}, $Vars = @{}, $Registry = @{})

  $mergedFacts = @{}
  foreach ($key in $Facts.Keys) { $mergedFacts[$key] = $Facts[$key] }
  foreach ($key in $UserFacts.Keys) { $mergedFacts[$key] = $UserFacts[$key] }

  $flat = @{}
  $flat['facts'] = $mergedFacts
  foreach ($key in $PackageVars.Keys) { $flat[$key] = $PackageVars[$key] }
  foreach ($key in $Vars.Keys) { $flat[$key] = $Vars[$key] }
  foreach ($key in $Registry.Keys) { $flat[$key] = $Registry[$key] }
  return $flat
}

function Copy-DeepData {
  # Recursively clones hashtables/ordered-dictionaries/lists produced by
  # ConvertFrom-Yaml (or built in-memory the same way). Needed for looped
  # tasks ('with'/'items' in Tasks.psm1): each iteration gets its own
  # independent copy of the task template to run '${{ item.* }}'
  # substitution against, rather than every iteration mutating the same
  # shared object in place.
  param($Data)

  if ($Data -is [string]) { return $Data }

  if ($Data -is [System.Collections.IDictionary]) {
    $copy = [ordered]@{}
    foreach ($key in $Data.Keys) { $copy[$key] = Copy-DeepData -Data $Data[$key] }
    return $copy
  }

  if ($Data -is [System.Collections.IList]) {
    $copy = [System.Collections.Generic.List[object]]::new()
    foreach ($element in $Data) { $copy.Add((Copy-DeepData -Data $element)) }
    # ',' prevents PowerShell from unrolling this across the function return
    # boundary - see the same note in Templates.psm1's Expand-TemplateNode.
    return ,$copy.ToArray()
  }

  return $Data
}

function Test-TagsMatch {
  # Flat tag matching for '-Tags': no filter means "include everything";
  # otherwise a leaf's effective tags (its own + every ancestor task's,
  # accumulated by Tasks.psm1) must intersect the requested tags.
  param([string[]] $Tags, [string[]] $Filter)
  if (-not $Filter -or $Filter.Count -eq 0) { return $true }
  foreach ($tag in $Filter) { if ($Tags -contains $tag) { return $true } }
  return $false
}

# --- 'creates' glob helpers (shared by zip and shell) ---------------------

function Resolve-CreatesPatterns {
  # Expands each 'creates' entry into concrete paths (handles globs).
  param($Creates)
  if (-not $Creates) { return @() }
  $results = [System.Collections.Generic.List[string]]::new()
  foreach ($pattern in @($Creates)) {
    $resolved = Resolve-UserPath $pattern
    if ($resolved -match '[*?]') {
      $parent = Split-Path $resolved -Parent
      $leaf   = Split-Path $resolved -Leaf
      if (Test-Path $parent) {
        Get-ChildItem -Path $parent -Filter $leaf -ErrorAction SilentlyContinue |
          ForEach-Object { $results.Add($_.FullName) }
      }
    } else {
      $results.Add($resolved)
    }
  }
  return $results.ToArray()
}

function Test-CreatesPresent {
  # True when every 'creates' pattern resolves to at least one existing path.
  # An empty/absent 'creates' list means "can't tell" -> always not-installed.
  param($Creates)
  if (-not $Creates -or @($Creates).Count -eq 0) { return $false }
  foreach ($pattern in @($Creates)) {
    $resolved = Resolve-UserPath $pattern
    if ($resolved -match '[*?]') {
      $parent = Split-Path $resolved -Parent
      $leaf   = Split-Path $resolved -Leaf
      if (-not (Test-Path $parent)) { return $false }
      $found = Get-ChildItem -Path $parent -Filter $leaf -ErrorAction SilentlyContinue
      if (-not $found -or @($found).Count -eq 0) { return $false }
    } else {
      if (-not (Test-Path $resolved)) { return $false }
    }
  }
  return $true
}

function Remove-CreatesPatterns {
  param($Creates)
  if (-not $Creates) { return }
  foreach ($pattern in @($Creates)) {
    $resolved = Resolve-UserPath $pattern
    if ($resolved -match '[*?]') {
      $parent = Split-Path $resolved -Parent
      $leaf   = Split-Path $resolved -Leaf
      Get-ChildItem -Path $parent -Filter $leaf -ErrorAction SilentlyContinue |
        Remove-Item -Force -ErrorAction SilentlyContinue
    } else {
      if (Test-Path $resolved) { Remove-Item -Path $resolved -Force -Recurse -ErrorAction SilentlyContinue }
    }
  }
}

Export-ModuleMember -Function *
