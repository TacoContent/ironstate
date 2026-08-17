#!/usr/bin/env pwsh
<#
.SYNOPSIS
  Handler for the 'go' group (Go binaries via 'go install').
#>

Set-StrictMode -Version Latest

Import-Module (Join-Path $PSScriptRoot '..\Common.psm1')

$script:GoBinDirCache = $null

function Get-GoBinDir {
  if ($script:GoBinDirCache) { return $script:GoBinDirCache }

  $gobin = (& go env GOBIN 2>$null | Out-String).Trim()
  if ($gobin) { $script:GoBinDirCache = $gobin; return $gobin }

  $gopath = (& go env GOPATH 2>$null | Out-String).Trim()
  if (-not $gopath) { $gopath = Join-Path $HOME 'go' }
  $script:GoBinDirCache = Join-Path $gopath 'bin'
  return $script:GoBinDirCache
}

function Get-GoBinaryPath {
  param($Item)
  $package = Get-Prop $Item 'package'
  $binName = (Split-Path -Leaf $package) + '.exe'
  Join-Path (Get-GoBinDir) $binName
}

function Get-GoHandler {
  [PSCustomObject]@{
    Test      = {
      param($Item)
      Test-Path (Get-GoBinaryPath -Item $Item)
    }
    Describe  = {
      param($Item, $Action)
      $package = Get-Prop $Item 'package'
      $version = Get-Prop $Item 'version' 'latest'
      if ($Action -eq 'Uninstall') { "remove $(Get-GoBinaryPath -Item $Item)" } else { "go install $package@$version" }
    }
    Install   = {
      param($Item)
      $package = Get-Prop $Item 'package'
      $version = Get-Prop $Item 'version' 'latest'
      $result = Invoke-ExternalCommand -Exe 'go' -Arguments @('install', "$package@$version")
      if ($result.rc -ne 0) { Write-Warning "go install $package@$version exited with code $($result.rc)" }
      return $result
    }
    Uninstall = {
      param($Item)
      Remove-Item -Path (Get-GoBinaryPath -Item $Item) -Force -ErrorAction SilentlyContinue
    }
  }
}

Export-ModuleMember -Function Get-GoHandler
