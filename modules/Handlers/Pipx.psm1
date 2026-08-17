#!/usr/bin/env pwsh
<#
.SYNOPSIS
  Handler for the 'pipx' group (Python isolated tools).
#>

Set-StrictMode -Version Latest

Import-Module (Join-Path $PSScriptRoot '..\Common.psm1')

function Get-PipxHandler {
  [PSCustomObject]@{
    Test      = {
      param($Item)
      $package = Get-Prop $Item 'package'
      $out = & pipx list --short 2>&1 | Out-String
      $match = ($out -split "`n") | Where-Object { $_.Trim().StartsWith("$package ") } | Select-Object -First 1
      return [bool] $match
    }
    Describe  = {
      param($Item, $Action)
      $package = Get-Prop $Item 'package'
      $state = Get-ItemState -Item $Item
      if ($Action -eq 'Uninstall') { "pipx uninstall $package" }
      elseif ($state -eq 'latest') { "pipx upgrade $package (installing if missing)" }
      else { "pipx install $package" }
    }
    Install   = {
      param($Item)
      $package = Get-Prop $Item 'package'
      $result =
        if ((Get-ItemState -Item $Item) -eq 'latest') {
          $upgradeResult = Invoke-ExternalCommand -Exe 'pipx' -Arguments @('upgrade', $package)
          if ($upgradeResult.rc -ne 0) { Invoke-ExternalCommand -Exe 'pipx' -Arguments @('install', $package) } else { $upgradeResult }
        } else {
          Invoke-ExternalCommand -Exe 'pipx' -Arguments @('install', $package)
        }
      if ($result.rc -ne 0) { Write-Warning "pipx install/upgrade $package exited with code $($result.rc)" }
      return $result
    }
    Uninstall = {
      param($Item)
      $package = Get-Prop $Item 'package'
      $result = Invoke-ExternalCommand -Exe 'pipx' -Arguments @('uninstall', $package)
      if ($result.rc -ne 0) { Write-Warning "pipx uninstall $package exited with code $($result.rc)" }
      return $result
    }
  }
}

Export-ModuleMember -Function Get-PipxHandler
