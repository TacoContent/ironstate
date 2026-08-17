#!/usr/bin/env pwsh
<#
.SYNOPSIS
  Handler for the 'shell' group: runs an inline command or a script file
  through a configurable interpreter ('host').

.DESCRIPTION
  Exactly one of 'command' (inline, multiline via a YAML block scalar) or
  'script' (a file, resolved relative to the install system directory at
  load time) should be given.

  'host' picks what processes it, like a shebang line: default 'pwsh' runs
  the script directly, in-process (no subprocess). Recognized presets
  (powershell, cmd, bash, sh, node, python) expand to their executable.
  Anything else is split on whitespace and used as-is - exe plus leading
  args - so any script runner on PATH works without code changes, e.g.
  'host: npx tsx' to run a TypeScript 'command'/'script'. 'extension'
  overrides the temp file's extension for inline 'command' under a
  non-'pwsh' host (defaults to a sensible one per preset, '.txt' otherwise)
  - e.g. 'extension: .ts' so 'npx tsx' sees a real TypeScript file.

  Output is captured rather than streamed live: stdout (including
  Write-Host/Write-Information, merged in via '6>&1' - verified empirically
  - alongside real stdout/Write-Output) and stderr are captured, then echoed
  after the command finishes (Write-Host for stdout, Write-Warning for
  stderr). Install/Uninstall return this result directly
  (rc/stdout/stdout_lines/stderr/stderr_lines) - ironstate.ps1's dispatch loop
  normalizes every handler's return value the same way, so this is what
  backs an 'id'-registered result for a shell task (see Common.psm1's
  Merge-FlatContext / ironstate.ps1's dispatch loop), same as any other
  module.

  Under the default 'pwsh' host specifically, output is captured by
  *variable assignment* rather than file redirection, so real pipeline
  objects (e.g. Get-ItemProperty's result) keep their actual .NET type
  instead of being flattened to text - verified empirically. When the
  command's non-Write-Host/-Information output is exactly one such object,
  its own properties are merged directly into the registered result
  alongside rc/stdout/etc. (never overwriting those reserved names), so
  e.g. '${{ pf.ProgramFilesDir }}' works directly off an 'id: pf' shell task
  that ran '(Get-ItemProperty -Path "HKLM:\...\CurrentVersion")' - no
  '.native'/'.result' indirection needed. Non-'pwsh' hosts have no such
  concept (an external process only ever produces text) and keep the
  simpler file-redirection capture.

  'creates' works like the zip group's 'creates': its presence signals
  "already run", and for state 'absent' those paths are removed instead of
  re-running anything. If 'creates' is omitted, the command always runs for
  state 'present'/'latest', and 'absent' has nothing to remove. 'creates' is
  always read from the top level, regardless of any per-state block below -
  it's the one signal Test-CreatesPresent needs before a target state is
  even chosen.

  Per-state command/script (scripted install/uninstall): 'command'/'script'/
  'args'/'host'/'extension' can each be nested one level deeper, under
  'present'/'absent'/'latest' keys, to run a different command per state
  instead of one command for every state:

    shell:
      present: { command: ./install.ps1 }
      absent:  { command: ./uninstall.ps1 }

  Resolve-ShellStateConfig picks the block matching the item's own 'state'
  (default 'present') and, field by field, falls back to the flat top-level
  fields for anything that block doesn't set itself - so an item can still
  just write 'command'/'script' at the top level with no per-state blocks
  at all, which is exactly the pre-existing single-command shape. 'absent'
  is the one exception with no such fallback: since the legacy behavior for
  state 'absent' has always been "remove 'creates' entries, run nothing",
  reusing the top-level (present-oriented) command for it by default would
  be a surprising behavior change - a dedicated 'absent' block is the only
  way to run a command on uninstall.
#>

Set-StrictMode -Version Latest

Import-Module (Join-Path $PSScriptRoot '..\Common.psm1')

# Preset 'host' names -> [executable, ...leading args]. 'pwsh' isn't listed
# here - it's special-cased to run the script directly, in-process.
$script:HostPresets = @{
  powershell = @('powershell.exe')
  cmd        = @('cmd.exe', '/d', '/c')
  bash       = @('bash.exe')
  sh         = @('sh.exe')
  node       = @('node.exe')
  python     = @('python.exe')
}

# Default temp-file extension per preset, used when 'command' (not 'script')
# is given and 'extension' isn't set explicitly.
$script:HostExtensions = @{
  powershell = '.ps1'
  cmd        = '.cmd'
  bash       = '.sh'
  sh         = '.sh'
  node       = '.js'
  python     = '.py'
}

function Get-ShellHostInvocation {
  param([string] $HostSpec)
  if ($HostSpec -eq 'pwsh') { return @() }
  if ($script:HostPresets.Contains($HostSpec)) { return @($script:HostPresets[$HostSpec]) }
  return @($HostSpec -split '\s+' | Where-Object { $_ })
}

function Get-ShellExitCode {
  # $LASTEXITCODE is only set once a native executable runs, and is a
  # session-global left over from whatever last set it - reset before every
  # invocation (a pure-cmdlet command that makes no native/exit call would
  # otherwise silently inherit a *stale* code from a previous, unrelated
  # shell task in the same run). Set-StrictMode throws on reading an
  # automatic variable that's never been assigned in this scope, hence the
  # Get-Variable guard rather than a bare '$LASTEXITCODE' read.
  $rc = Get-Variable -Name LASTEXITCODE -ValueOnly -ErrorAction SilentlyContinue
  if ($null -eq $rc) { return 0 }
  return $rc
}

function Merge-ShellNativeResult {
  # Merges a captured native object's own properties into the result
  # hashtable, without ever overwriting the reserved rc/stdout/etc. fields -
  # this is what makes e.g. '${{ pf.ProgramFilesDir }}' work directly off an
  # 'id: pf' shell task's registered result.
  param([hashtable] $Result, $NativeObject)

  $props =
    if ($NativeObject -is [System.Collections.IDictionary]) { $NativeObject }
    else {
      $bag = @{}
      foreach ($p in $NativeObject.PSObject.Properties) { $bag[$p.Name] = $p.Value }
      $bag
    }

  foreach ($key in $props.Keys) {
    if (-not $Result.Contains($key)) { $Result[$key] = $props[$key] }
  }
}

function Invoke-ShellInProcess {
  # Captures via variable assignment (not file redirection) so real pipeline
  # objects (e.g. Get-ItemProperty's result) keep their actual .NET type -
  # see the module docstring.
  param([string] $RunPath, [string[]] $ItemArgs)

  $stderrFile = [System.IO.Path]::Combine([System.IO.Path]::GetTempPath(), [System.IO.Path]::GetRandomFileName() + '.err')
  try {
    $global:LASTEXITCODE = $null
    $raw = @(& $RunPath @ItemArgs 6>&1 2>$stderrFile)
    $rc = Get-ShellExitCode
    $stderrLines = @(if (Test-Path $stderrFile) { @(Get-Content -Path $stderrFile) } else { @() })
  } finally {
    Remove-Item -Path $stderrFile -Force -ErrorAction SilentlyContinue
  }

  $dataObjects = @($raw | Where-Object { $_ -isnot [System.Management.Automation.InformationRecord] })

  # Reconstructs the text a live terminal would have shown, in original
  # order (unwrapping InformationRecord to its message, formatting anything
  # else the same way the console default formatter would).
  $displaySequence = @($raw | ForEach-Object { if ($_ -is [System.Management.Automation.InformationRecord]) { $_.MessageData } else { $_ } })
  $stdout = (($displaySequence | Out-String)).TrimEnd("`r", "`n")
  $stdoutLines = if ($stdout -eq '') { @() } else { @($stdout -split "`r?`n") }

  $result = @{
    rc           = $rc
    stdout       = $stdout
    stdout_lines = $stdoutLines
    stderr       = ($stderrLines -join "`n")
    stderr_lines = $stderrLines
  }

  if ($dataObjects.Count -eq 1) {
    $obj = $dataObjects[0]
    if ($null -ne $obj -and $obj -isnot [string] -and $obj -isnot [System.ValueType]) {
      Merge-ShellNativeResult -Result $result -NativeObject $obj
    }
  }

  foreach ($line in $stdoutLines) { Write-Host $line }
  foreach ($line in $stderrLines) { Write-Warning $line }

  return $result
}

function Invoke-ShellExternal {
  # External hosts have no "object" concept - a separate process only ever
  # produces text - so this is just Common.psm1's shared external-command
  # capture, same as every CLI-backed package manager handler uses.
  param([string] $Exe, [string[]] $LeadingArgs, [string] $RunPath, [string[]] $ItemArgs)
  return Invoke-ExternalCommand -Exe $Exe -Arguments (@($LeadingArgs) + @($RunPath) + @($ItemArgs))
}

function Resolve-ShellStateConfig {
  # Effective command/script/args/host/extension for a given state - see
  # the module docstring's "Per-state command/script" section for the
  # fallback rules (per-field, top-level, except 'absent' has none).
  param($Item, [Parameter(Mandatory)][string] $State)

  $block = Get-Prop $Item $State
  if ($block -isnot [System.Collections.IDictionary]) { $block = $null }
  $fallback = if ($State -eq 'absent') { $null } else { $Item }

  [PSCustomObject]@{
    Command   = if ($block -and $block.Contains('command')) { $block['command'] } else { Get-Prop $fallback 'command' }
    Script    = if ($block -and $block.Contains('script')) { $block['script'] } else { Get-Prop $fallback 'script' }
    ItemArgs  = @(if ($block -and $block.Contains('args')) { $block['args'] } else { Get-Prop $fallback 'args' @() })
    HostSpec  = if ($block -and $block.Contains('host')) { $block['host'] } else { Get-Prop $fallback 'host' 'pwsh' }
    Extension = if ($block -and $block.Contains('extension')) { $block['extension'] } else { Get-Prop $fallback 'extension' }
  }
}

function Get-ShellItemLabel {
  # Get-ItemLabel falls back to a bare-top-level 'command'/'script', which a
  # shell item defined entirely through per-state blocks doesn't have -
  # check those blocks too before giving up to '<unknown>'. '-State', when
  # given, is checked first so e.g. describing an uninstall prefers the
  # 'absent' block's own command over whichever block happens to be first.
  param($Item, [string] $State)
  $label = Get-ItemLabel -Item $Item
  if ($label -ne '<unknown>') { return $label }
  $statesToCheck = @($State) + @('present', 'absent', 'latest') | Where-Object { $_ } | Select-Object -Unique
  foreach ($state in $statesToCheck) {
    $block = Get-Prop $Item $state
    if ($block -is [System.Collections.IDictionary]) {
      $nested = Get-ItemLabel -Item $block
      if ($nested -ne '<unknown>') { return $nested }
    }
  }
  return '<unknown>'
}

function Invoke-ShellItem {
  param($Config, [Parameter(Mandatory)][string] $Label)

  $command    = $Config.Command
  $scriptPath = $Config.Script
  $itemArgs   = @($Config.ItemArgs)
  $hostSpec   = $Config.HostSpec

  if (-not $scriptPath -and -not $command) {
    Write-Warning "Shell item '$Label' has neither 'command' nor 'script'"
    return Get-ZeroExecResult
  }

  $invocation = @(Get-ShellHostInvocation -HostSpec $hostSpec)

  $runPath  = $scriptPath
  $tempFile = $null
  if (-not $runPath) {
    $extension =
      if ($hostSpec -eq 'pwsh') { '.ps1' }
      else {
        $explicit = $Config.Extension
        if ($explicit) { $explicit }
        elseif ($script:HostExtensions.Contains($hostSpec)) { $script:HostExtensions[$hostSpec] }
        else { '.txt' }
      }
    $tempFile = [System.IO.Path]::Combine([System.IO.Path]::GetTempPath(), [System.IO.Path]::GetRandomFileName() + $extension)
    Set-Content -Path $tempFile -Value $command
    $runPath = $tempFile
  } elseif (-not (Test-Path $runPath)) {
    Write-Warning "Shell script not found: $runPath"
    return Get-ZeroExecResult
  }

  try {
    $result =
      if ($invocation.Count -eq 0) { Invoke-ShellInProcess -RunPath $runPath -ItemArgs $itemArgs }
      else { Invoke-ShellExternal -Exe $invocation[0] -LeadingArgs @($invocation | Select-Object -Skip 1) -RunPath $runPath -ItemArgs $itemArgs }
  } finally {
    if ($tempFile) { Remove-Item -Path $tempFile -Force -ErrorAction SilentlyContinue }
  }

  if ($result.rc -and $result.rc -ne 0) {
    Write-Warning "Shell item '$Label' exited with code $($result.rc)"
  }

  return $result
}

function Get-ShellHandler {
  [PSCustomObject]@{
    Test      = {
      param($Item)
      Test-CreatesPresent -Creates (Get-Prop $Item 'creates')
    }
    Describe  = {
      param($Item, $Action)
      if ($Action -eq 'Uninstall') {
        $label = Get-ShellItemLabel -Item $Item -State 'absent'
        $config = Resolve-ShellStateConfig -Item $Item -State 'absent'
        if ($config.Command -or $config.Script) { "run shell '$label' via '$($config.HostSpec)' (uninstall)" }
        else { "remove creates entries for shell '$label'" }
      } else {
        $state = Get-ItemState -Item $Item
        $label = Get-ShellItemLabel -Item $Item -State $state
        $config = Resolve-ShellStateConfig -Item $Item -State $state
        "run shell '$label' via '$($config.HostSpec)'"
      }
    }
    Install   = {
      param($Item)
      $state = Get-ItemState -Item $Item
      $config = Resolve-ShellStateConfig -Item $Item -State $state
      return Invoke-ShellItem -Config $config -Label (Get-ShellItemLabel -Item $Item -State $state)
    }
    Uninstall = {
      param($Item)
      $config = Resolve-ShellStateConfig -Item $Item -State 'absent'
      $result =
        if ($config.Command -or $config.Script) { Invoke-ShellItem -Config $config -Label (Get-ShellItemLabel -Item $Item -State 'absent') }
        else { Get-ZeroExecResult }
      Remove-CreatesPatterns -Creates (Get-Prop $Item 'creates')
      return $result
    }
  }
}

Export-ModuleMember -Function Get-ShellHandler
