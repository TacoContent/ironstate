#!/usr/bin/env pwsh
<#
.SYNOPSIS
  EPS-style template engine - shells out to the real PowerShell Gallery
  'EPS' module (straightdave/eps): '<%= %>' (output), '<%# %>' (comment),
  and '<% %>' (scriptlet - arbitrary statements, e.g. control flow) tags.

.DESCRIPTION
  Unlike the 'jinja'/'herestring' engines (both backed by ironstate's own
  sandboxed expression grammar), this engine runs real, unrestricted
  PowerShell inside '<% %>' tags - that's what EPS fundamentally is, and
  there is no sandboxed mode in the upstream module to opt into. Its
  '-Safe' switch only isolates the template's variable scope into a
  separate thread (so it can't see or pollute the caller's own variables)
  - it does not restrict what a template's code can do. Use this engine
  only for trusted template content, matching this repo's own trust level
  everywhere else (host-config YAML + scriptblocks are already full
  PowerShell).

  Installs the 'EPS' module from the PowerShell Gallery on first use if
  it isn't already present (Install-Module -Scope CurrentUser -Force) - a
  real, one-time side effect worth expecting on a fresh machine.
#>

Set-StrictMode -Version Latest

function Test-EpsModuleAvailable {
  return [bool] (Get-Module -ListAvailable -Name EPS)
}

function Install-EpsModuleIfMissing {
  if (-not (Test-EpsModuleAvailable)) {
    Write-Host "Installing PowerShell Gallery module 'EPS' (required by template engine: eps)..."
    Install-Module -Name EPS -Scope CurrentUser -Force
  }
  Import-Module EPS -Force
}

function Render-EpsTemplate {
  param([Parameter(Mandatory)][string] $Content, [Parameter(Mandatory)][hashtable] $Context)
  Install-EpsModuleIfMissing
  return Invoke-EpsTemplate -Template $Content -Binding $Context -Safe
}

Export-ModuleMember -Function Render-EpsTemplate
