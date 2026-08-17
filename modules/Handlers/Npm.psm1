#!/usr/bin/env pwsh
<#
.SYNOPSIS
  Handler for the 'npm' group (Node global packages).
#>

Set-StrictMode -Version Latest

Import-Module (Join-Path $PSScriptRoot '..\Common.psm1')

function Get-NpmHandler {
  [PSCustomObject]@{
    Test      = {
      param($Item)
      $package = Get-Prop $Item 'package'
      & npm list -g $package --depth=0 *> $null
      return $LASTEXITCODE -eq 0
    }
    Describe  = {
      param($Item, $Action)
      $package = Get-Prop $Item 'package'
      if ($Action -eq 'Uninstall') { "npm uninstall -g $package" } else { "npm install -g $package" }
    }
    Install   = {
      param($Item)
      $package = Get-Prop $Item 'package'
      $result = Invoke-ExternalCommand -Exe 'npm' -Arguments @('install', '-g', $package)
      if ($result.rc -ne 0) { Write-Warning "npm install -g $package exited with code $($result.rc)" }
      return $result
    }
    Uninstall = {
      param($Item)
      $package = Get-Prop $Item 'package'
      $result = Invoke-ExternalCommand -Exe 'npm' -Arguments @('uninstall', '-g', $package)
      if ($result.rc -ne 0) { Write-Warning "npm uninstall -g $package exited with code $($result.rc)" }
      return $result
    }
  }
}

Export-ModuleMember -Function Get-NpmHandler
