#!/usr/bin/env pwsh
<#
.SYNOPSIS
  Handler for the 'chocolatey' group.
#>

Set-StrictMode -Version Latest

Import-Module (Join-Path $PSScriptRoot '..\Common.psm1')

function Get-ChocolateyHandler {
  [PSCustomObject]@{
    Test      = {
      param($Item)
      $package = Get-Prop $Item 'package'
      $out = & choco list --local-only $package --exact --limit-output 2>&1 | Out-String
      return -not [string]::IsNullOrWhiteSpace($out)
    }
    Describe  = {
      param($Item, $Action)
      $package = Get-Prop $Item 'package'
      $state = Get-ItemState -Item $Item
      if ($Action -eq 'Uninstall') { "choco uninstall $package -y" }
      elseif ($state -eq 'latest') { "choco upgrade $package -y" }
      else { "choco install $package -y" }
    }
    Install   = {
      param($Item)
      $package = Get-Prop $Item 'package'
      $state = Get-ItemState -Item $Item
      $version = if ($state -ne 'latest') { Get-Prop $Item 'version' } else { $null }
      $verb = if ($state -eq 'latest') { 'upgrade' } else { 'install' }
      $versionArgs = if ($version) { @("--version=$version") } else { @() }
      $result = Invoke-ExternalCommand -Exe 'choco' -Arguments (@($verb, $package, '-y', '--accept-license') + $versionArgs)
      if ($result.rc -ne 0) { Write-Warning "choco $verb $package exited with code $($result.rc)" }
      return $result
    }
    Uninstall = {
      param($Item)
      $package = Get-Prop $Item 'package'
      $result = Invoke-ExternalCommand -Exe 'choco' -Arguments @('uninstall', $package, '-y')
      if ($result.rc -ne 0) { Write-Warning "choco uninstall $package exited with code $($result.rc)" }
      return $result
    }
  }
}

Export-ModuleMember -Function Get-ChocolateyHandler
