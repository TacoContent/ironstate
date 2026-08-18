#!/usr/bin/env pwsh
<#
.SYNOPSIS
  Handler for the 'gem' group (Ruby Gems).
#>

Set-StrictMode -Version Latest

Import-Module (Join-Path $PSScriptRoot '..\Common.psm1')

function Get-RubyGemHandler {
  [PSCustomObject]@{
    Test      = {
      param($Item)
      $package = Get-Prop $Item 'package'
      & gem list $package -i *> $null
      return $LASTEXITCODE -eq 0
    }
    Describe  = {
      param($Item, $Action)
      $package = Get-Prop $Item 'package'
      $state = Get-ItemState -Item $Item
      $version = if ($state -ne 'latest') { Get-Prop $Item 'version' } else { $null }
      $versionArgs = if ($version) { "--version=$version" } else { "" }
      if ($Action -eq 'Uninstall') { "gem uninstall $package" }
      elseif ($state -eq 'latest') { "gem update $package" }
      else { "gem install $package $versionArgs".Trim() }
    }
    Install   = {
      param($Item)
      $package = Get-Prop $Item 'package'
      $state = Get-ItemState -Item $Item
      $version = if ($state -ne 'latest') { Get-Prop $Item 'version' } else { $null }
      $verb = if ($state -eq 'latest') { 'update' } else { 'install' }
      $versionArgs = if ($version) { @("--version=$version") } else { @() }
      $result = Invoke-ExternalCommand -Exe 'gem' -Arguments (@($verb, $package) + $versionArgs)
      if ($result.rc -ne 0) { Write-Warning "gem $verb $package exited with code $($result.rc)" }
      return $result
    }
    Uninstall = {
      param($Item)
      $package = Get-Prop $Item 'package'
      $result = Invoke-ExternalCommand -Exe 'gem' -Arguments @('uninstall', $package)
      if ($result.rc -ne 0) { Write-Warning "gem uninstall $package exited with code $($result.rc)" }
      return $result
    }
  }
}

Export-ModuleMember -Function Get-RubyGemHandler
