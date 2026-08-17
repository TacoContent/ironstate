#!/usr/bin/env pwsh
<#
.SYNOPSIS
  Handler for the 'cargo' group (Rust crates).
#>

Set-StrictMode -Version Latest

Import-Module (Join-Path $PSScriptRoot '..\Common.psm1')

function Get-CargoHandler {
  [PSCustomObject]@{
    Test      = {
      param($Item)
      $package = Get-Prop $Item 'package'
      $out = & cargo install --list 2>&1 | Out-String
      return $out -match "(?m)^$([regex]::Escape($package))\s+v"
    }
    Describe  = {
      param($Item, $Action)
      $package = Get-Prop $Item 'package'
      if ($Action -eq 'Uninstall') { "cargo uninstall $package" } else { "cargo install $package --force" }
    }
    Install   = {
      param($Item)
      $package = Get-Prop $Item 'package'
      $result = Invoke-ExternalCommand -Exe 'cargo' -Arguments @('install', $package, '--force')
      if ($result.rc -ne 0) { Write-Warning "cargo install $package exited with code $($result.rc)" }
      return $result
    }
    Uninstall = {
      param($Item)
      $package = Get-Prop $Item 'package'
      $result = Invoke-ExternalCommand -Exe 'cargo' -Arguments @('uninstall', $package)
      if ($result.rc -ne 0) { Write-Warning "cargo uninstall $package exited with code $($result.rc)" }
      return $result
    }
  }
}

Export-ModuleMember -Function Get-CargoHandler
