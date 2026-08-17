#!/usr/bin/env pwsh
<#
.SYNOPSIS
  Handler for the 'eget' group (GitHub release binaries via eget).
#>

Set-StrictMode -Version Latest

Import-Module (Join-Path $PSScriptRoot '..\Common.psm1')

function Get-EgetExpandedArgs {
  param($Item)
  $eargs = Get-Prop $Item 'args'
  if (-not $eargs) { return @() }
  return @($eargs) | ForEach-Object {
    if ($_ -match '^(--to=)(.+)$') { "$($Matches[1])$(Resolve-UserPath $Matches[2])" } else { $_ }
  }
}

function Get-EgetTargetPath {
  param($Item)
  $toArg = @(Get-EgetExpandedArgs -Item $Item) | Where-Object { $_ -match '^--to=(.+)$' } | Select-Object -First 1
  if (-not $toArg) { return $null }
  return ($toArg -replace '^--to=', '')
}

function Get-EgetHandler {
  [PSCustomObject]@{
    Test      = {
      param($Item)
      $target = Get-EgetTargetPath -Item $Item
      if (-not $target) { return $false }
      Test-Path $target
    }
    Describe  = {
      param($Item, $Action)
      $package = Get-Prop $Item 'package'
      if ($Action -eq 'Uninstall') { "remove $(Get-EgetTargetPath -Item $Item)" }
      else { "eget $package $((Get-EgetExpandedArgs -Item $Item) -join ' ')" }
    }
    Install   = {
      param($Item)
      $package = Get-Prop $Item 'package'
      $result = Invoke-ExternalCommand -Exe 'eget' -Arguments (@($package) + @(Get-EgetExpandedArgs -Item $Item))
      if ($result.rc -ne 0) { Write-Warning "eget $package exited with code $($result.rc)" }
      return $result
    }
    Uninstall = {
      param($Item)
      $target = Get-EgetTargetPath -Item $Item
      if ($target) { Remove-Item -Path $target -Force -ErrorAction SilentlyContinue }
    }
  }
}

Export-ModuleMember -Function Get-EgetHandler
