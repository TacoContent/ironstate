#!/usr/bin/env pwsh
<#
.SYNOPSIS
  Handler for the 'copy' group: copies a local file - or a whole directory,
  recursively - into place.

.DESCRIPTION
  'src' is resolved relative to the install system directory (install/windows,
  or the owning package's own directory for packages/<name>/main.yml) by
  Resolve-RelativePathsInPlace at load time - by the time this handler runs,
  'src' is already an absolute path.

  If 'src' is a directory, every file under it is copied into 'dest',
  recursively, preserving the relative subdirectory structure - rsync/
  Ansible style: a trailing '/' on 'src' (e.g. 'files/custom/') copies its
  *contents* directly into 'dest', while no trailing slash (e.g.
  'files/custom') nests it as 'dest/custom/...' instead. 'dest' is created
  as a directory either way.

  "installed" for a directory copy means every file under 'src' already
  exists at its corresponding path under 'dest' with a matching SHA256 hash
  - same convention as a single file, just checked per-file. It does *not*
  mean 'dest' is an exact mirror: extra files already in 'dest' that aren't
  under 'src' are left alone, both by the install check and by 'absent'.

  'absent' removes only the files this task would have copied (not all of
  'dest'), then cleans up any subdirectories under 'dest' that are now
  empty as a result - 'dest' itself is never removed, since especially in
  the trailing-slash "contents" form it may be a directory other tasks or
  content share. This does mean 'absent' can't identify what to remove if
  'src' no longer exists by the time it runs (nothing to re-derive the
  copied file list from) - same limitation a single-file copy already has,
  since its own Test also depends on 'src' still being there to hash.
#>

Set-StrictMode -Version Latest

Import-Module (Join-Path $PSScriptRoot '..\Common.psm1')

function Test-CopySrcIsDirectory {
  param([string] $Src)
  return (Test-Path -LiteralPath $Src -PathType Container)
}

function Get-CopyDestRoot {
  # Trailing '/' or '\' on 'src' -> copy its *contents* into 'dest' directly;
  # no trailing slash -> nest it as 'dest/<src's own folder name>/...'.
  param([string] $Src, [string] $Dest)
  if ($Src.EndsWith('/') -or $Src.EndsWith('\')) { return $Dest }
  return Join-Path $Dest (Split-Path $Src.TrimEnd('/', '\') -Leaf)
}

function Get-CopyRelativeFiles {
  # Every file under 'src', as paths relative to 'src' itself (e.g.
  # 'aliases\dev.ps1'), so they can be replayed underneath a different root.
  param([string] $Src)
  $root = (Resolve-Path -LiteralPath $Src.TrimEnd('/', '\')).Path
  Get-ChildItem -Path $root -Recurse -File -Force | ForEach-Object {
    $_.FullName.Substring($root.Length).TrimStart('\', '/')
  }
}

function Test-CopyDirectoryPresent {
  param([string] $Src, [string] $DestRoot)
  $relFiles = @(Get-CopyRelativeFiles -Src $Src)
  if ($relFiles.Count -eq 0) { return (Test-Path $DestRoot -PathType Container) }

  $root = $Src.TrimEnd('/', '\')
  foreach ($rel in $relFiles) {
    $destFile = Join-Path $DestRoot $rel
    if (-not (Test-Path $destFile -PathType Leaf)) { return $false }
    $srcHash  = (Get-FileHash -Path (Join-Path $root $rel) -Algorithm SHA256).Hash
    $destHash = (Get-FileHash -Path $destFile -Algorithm SHA256).Hash
    if ($srcHash -ne $destHash) { return $false }
  }
  return $true
}

function Install-CopyDirectory {
  param([string] $Src, [string] $DestRoot)
  if (-not (Test-Path $DestRoot)) { New-Item -ItemType Directory -Path $DestRoot -Force | Out-Null }

  $root = $Src.TrimEnd('/', '\')
  foreach ($rel in (Get-CopyRelativeFiles -Src $Src)) {
    $destFile = Join-Path $DestRoot $rel
    $destFileDir = Split-Path $destFile -Parent
    if ($destFileDir -and -not (Test-Path $destFileDir)) { New-Item -ItemType Directory -Path $destFileDir -Force | Out-Null }
    Copy-Item -Path (Join-Path $root $rel) -Destination $destFile -Force
  }
}

function Uninstall-CopyDirectory {
  # Removes only the files 'src' would have copied, then prunes any
  # subdirectories under 'dest' left empty by that - never 'dest' itself.
  param([string] $Src, [string] $DestRoot)

  $touchedDirs = [System.Collections.Generic.HashSet[string]]::new()
  foreach ($rel in (Get-CopyRelativeFiles -Src $Src)) {
    $destFile = Join-Path $DestRoot $rel
    if (Test-Path $destFile) { Remove-Item -Path $destFile -Force }
    $parent = Split-Path $destFile -Parent
    while ($parent -and $parent.Length -gt $DestRoot.Length) {
      [void] $touchedDirs.Add($parent)
      $parent = Split-Path $parent -Parent
    }
  }

  foreach ($dir in ($touchedDirs | Sort-Object { $_.Length } -Descending)) {
    if ((Test-Path $dir) -and -not (Get-ChildItem -Path $dir -Force | Select-Object -First 1)) {
      Remove-Item -Path $dir -Force
    }
  }
}

function Get-CopyHandler {
  [PSCustomObject]@{
    Test      = {
      param($Item)
      $dest = Resolve-UserPath (Get-Prop $Item 'dest')
      $src  = Get-Prop $Item 'src'
      if (-not (Test-Path $src)) { Write-Warning "Source path for copy does not exist: $src"; return $false }

      if (Test-CopySrcIsDirectory -Src $src) {
        return Test-CopyDirectoryPresent -Src $src -DestRoot (Get-CopyDestRoot -Src $src -Dest $dest)
      }
      if (-not (Test-Path $dest)) { return $false }
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

      if (Test-CopySrcIsDirectory -Src $src) {
        Install-CopyDirectory -Src $src -DestRoot (Get-CopyDestRoot -Src $src -Dest $dest)
        return
      }
      $destDir = Split-Path $dest -Parent
      if ($destDir -and -not (Test-Path $destDir)) { New-Item -ItemType Directory -Path $destDir -Force | Out-Null }
      Copy-Item -Path $src -Destination $dest -Force
    }
    Uninstall = {
      param($Item)
      $dest = Resolve-UserPath (Get-Prop $Item 'dest')
      $src  = Get-Prop $Item 'src'

      if ($src -and (Test-CopySrcIsDirectory -Src $src)) {
        Uninstall-CopyDirectory -Src $src -DestRoot (Get-CopyDestRoot -Src $src -Dest $dest)
        return
      }
      if (Test-Path $dest) { Remove-Item -Path $dest -Force }
    }
  }
}

Export-ModuleMember -Function Get-CopyHandler
