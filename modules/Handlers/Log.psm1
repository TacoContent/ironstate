#!/usr/bin/env pwsh
<#
.SYNOPSIS
  Handler for the 'log' action: prints a message at a chosen level.

.DESCRIPTION
  Reuses the existing present/absent/latest state machine (Common.psm1)
  instead of adding a second "always run" kind of module: Test reports
  "installed" exactly when 'state' is 'absent', so 'present'/'latest' always
  resolve to Install (prints the 'install' message) and 'absent' always
  resolves to Uninstall (prints the 'uninstall' message). Log has no real
  idempotent "already applied" concept - it always fires when reached; state
  only selects which phase's message prints.

  Accepts either the nested form:
    log:
      install: { message: "...", level: info }
      uninstall: { message: "...", level: warning }
  or a flat shorthand for the common "just print something" case:
    log:
      message: "..."
      level: info
  which is treated as { install: { message, level } }.

  'level' (case-insensitive, default 'info') maps to Write-Debug/
  Write-Verbose/Write-Host/Write-Warning/Write-Error. 'info' uses Write-Host
  to match this codebase's existing convention for normal operational lines.
#>

Set-StrictMode -Version Latest

Import-Module (Join-Path $PSScriptRoot '..\Common.psm1')

function Get-LogPhaseSpec {
  param($Item, [Parameter(Mandatory)][string] $Phase)

  $nested = Get-Prop $Item $Phase
  if ($nested) { return $nested }
  if ($Phase -eq 'install' -and -not (Get-Prop $Item 'install') -and -not (Get-Prop $Item 'uninstall')) {
    return $Item
  }
  return $null
}

function Write-LogMessage {
  param($Item, [Parameter(Mandatory)][string] $Phase)

  $spec = Get-LogPhaseSpec -Item $Item -Phase $Phase
  if (-not $spec) { return }

  $message = Get-Prop $spec 'message'
  if (-not $message) { Write-Warning "log action's '$Phase' phase has no 'message'"; return }

  switch ((Get-Prop $spec 'level' 'info').ToString().ToLowerInvariant()) {
    'debug'   { Write-Debug $message }
    'verbose' { Write-Verbose $message }
    'warning' { Write-Warning $message }
    'error'   { Write-Error $message }
    default   { Write-Host $message }
  }
}

function Get-LogHandler {
  [PSCustomObject]@{
    Test      = {
      param($Item)
      (Get-ItemState -Item $Item) -eq 'absent'
    }
    Describe  = {
      param($Item, $Action)
      $phase = if ($Action -eq 'Uninstall') { 'uninstall' } else { 'install' }
      $spec = Get-LogPhaseSpec -Item $Item -Phase $phase
      $message = if ($spec) { Get-Prop $spec 'message' } else { $null }
      if ($message) { "log ($phase): $message" } else { "log ($phase): <no message>" }
    }
    Install   = {
      param($Item)
      Write-LogMessage -Item $Item -Phase 'install'
    }
    Uninstall = {
      param($Item)
      Write-LogMessage -Item $Item -Phase 'uninstall'
    }
  }
}

Export-ModuleMember -Function Get-LogHandler
