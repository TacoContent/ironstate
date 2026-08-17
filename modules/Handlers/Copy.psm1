#!/usr/bin/env pwsh
<#
.SYNOPSIS
  Handler for the 'copy' group: copies a local file into place.

.DESCRIPTION
  'src' is resolved relative to the install system directory (install/windows,
  or the owning package's own directory for packages/<name>/main.yml) by
  Resolve-RelativePathsInPlace at load time - by the time this handler runs,
  'src' is already an absolute path.
#>

Set-StrictMode -Version Latest

Import-Module (Join-Path $PSScriptRoot '..\Common.psm1')

function Get-CopyHandler {
  [PSCustomObject]@{
    Test      = {
      param($Item)
      $dest = Resolve-UserPath (Get-Prop $Item 'dest')
      $src  = Get-Prop $Item 'src'
      if (-not (Test-Path $dest)) { return $false }
      if (-not (Test-Path $src)) { Write-Warning "Source path for copy does not exist: $src"; return $false }
      $srcHash  = (Get-FileHash -Path $src -Algorithm SHA256).Hash
      $destHash = (Get-FileHash -Path $dest -Algorithm SHA256).Hash
      return $srcHash -eq $destHash
    }
    Describe  = {
      param($Item, $Action)
      $dest = Resolve-UserPath (Get-Prop $Item 'dest')
      $src  = Get-Prop $Item 'src'
      if ($Action -eq 'Uninstall') { "remove $dest" } else { "copy $src -> $dest" }
    }
    Install   = {
      param($Item)
      $dest = Resolve-UserPath (Get-Prop $Item 'dest')
      $src  = Get-Prop $Item 'src'
      if (-not (Test-Path $src)) { Write-Warning "Source path for copy does not exist, skipping: $src"; return }
      $destDir = Split-Path $dest -Parent
      if ($destDir -and -not (Test-Path $destDir)) { New-Item -ItemType Directory -Path $destDir -Force | Out-Null }
      Copy-Item -Path $src -Destination $dest -Force
    }
    Uninstall = {
      param($Item)
      $dest = Resolve-UserPath (Get-Prop $Item 'dest')
      if (Test-Path $dest) { Remove-Item -Path $dest -Force }
    }
  }
}

Export-ModuleMember -Function Get-CopyHandler
