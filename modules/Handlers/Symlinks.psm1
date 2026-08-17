#!/usr/bin/env pwsh
<#
.SYNOPSIS
  Handler for the 'symlinks' group.
#>

Set-StrictMode -Version Latest

Import-Module (Join-Path $PSScriptRoot '..\Common.psm1')

function Get-SymlinksHandler {
  [PSCustomObject]@{
    Test      = {
      param($Item)
      $dest = Resolve-UserPath (Get-Prop $Item 'dest')
      if (-not (Test-Path $dest)) { return $false }
      $src = Resolve-UserPath (Get-Prop $Item 'src')
      if (-not (Test-Path $src)) { Write-Warning "Source path for symlink does not exist: $src"; return $false }
      $existing = Get-Item -Path $dest -Force
      return ($existing.LinkType -eq 'SymbolicLink') -and ($existing.Target -contains $src)
    }
    Describe  = {
      param($Item, $Action)
      $dest = Resolve-UserPath (Get-Prop $Item 'dest')
      $src = Resolve-UserPath (Get-Prop $Item 'src')
      if ($Action -eq 'Uninstall') { "remove symlink $dest" } else { "link $dest -> $src" }
    }
    Install   = {
      param($Item)
      $dest = Resolve-UserPath (Get-Prop $Item 'dest')
      $src = Resolve-UserPath (Get-Prop $Item 'src')
      if (-not (Test-Path $src)) { Write-Warning "Source path for symlink does not exist, skipping: $src"; return }
      if (Test-Path $dest) { Remove-Item -Path $dest -Force }
      New-Item -ItemType SymbolicLink -Path $dest -Target $src -Force | Out-Null
    }
    Uninstall = {
      param($Item)
      $dest = Resolve-UserPath (Get-Prop $Item 'dest')
      if (Test-Path $dest) { Remove-Item -Path $dest -Force }
    }
  }
}

Export-ModuleMember -Function Get-SymlinksHandler
