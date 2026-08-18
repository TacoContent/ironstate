#!/usr/bin/env pwsh
<#
.SYNOPSIS
  Handler for the 'winget' group (Windows Package Manager).
#>

Set-StrictMode -Version Latest

Import-Module (Join-Path $PSScriptRoot '..\Common.psm1')

function Get-WingetHandler {
  [PSCustomObject]@{
    Test      = {
      param($Item)
      $package = Get-Prop $Item 'package'
      $out = & winget list --id $package --exact --accept-source-agreements 2>&1 | Out-String
      return ($LASTEXITCODE -eq 0) -and ($out -notmatch 'No installed package found')
    }
    Describe  = {
      param($Item, $Action)
      $package = Get-Prop $Item 'package'
      $source = Get-Prop $Item 'source'
      $override = Get-Prop $Item 'override'
      if ($Action -eq 'Uninstall') { "winget uninstall --id $package --exact" }
      else { 
        "winget install --id $package --exact$(if ($source) { " --source $source" })$( if ($override) { " --override $override" })" 
      }
    }
    Install   = {
      param($Item)
      $package = Get-Prop $Item 'package'
      $override = Get-Prop $Item 'override' # --override args for winget
      $overrideArgs = if ($override) { @('--override', $override) } else { @() }
      $source = Get-Prop $Item 'source'
      $sourceArgs = if ($source) { @('--source', $source) } else { @() }
      $result = Invoke-ExternalCommand -Exe 'winget' -Arguments (@('install', '--id', $package, '--exact', '--accept-source-agreements', '--accept-package-agreements') + $sourceArgs + $overrideArgs)
      if ($result.rc -ne 0) { Write-Warning "winget install $package exited with code $($result.rc)" }
      return $result
    }
    Uninstall = {
      param($Item)
      $package = Get-Prop $Item 'package'
      $result = Invoke-ExternalCommand -Exe 'winget' -Arguments @('uninstall', '--id', $package, '--exact')
      if ($result.rc -ne 0) { Write-Warning "winget uninstall $package exited with code $($result.rc)" }
      return $result
    }
  }
}

Export-ModuleMember -Function Get-WingetHandler
