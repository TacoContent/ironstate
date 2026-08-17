#!/usr/bin/env pwsh
<#
.SYNOPSIS
  Handler for the 'zip' group (download + extract a ZIP archive directly).
#>

Set-StrictMode -Version Latest

Import-Module (Join-Path $PSScriptRoot '..\Common.psm1')

function Get-ZipSha256CachePath {
  param($Item)
  $sha256 = Get-Prop $Item 'sha256'
  if ($sha256 -and $sha256.Contains('cache') -and $sha256['cache']) {
    return Resolve-UserPath $sha256['cache']
  }
  $dest = Resolve-UserPath (Get-Prop $Item 'dest')
  $src = Get-Prop $Item 'src'
  $filename = [System.IO.Path]::GetFileName(([uri]$src).LocalPath)
  return Join-Path $dest "$filename.sha256"
}

function Invoke-ZipDownloadAndExtract {
  param($Item)

  $src     = Get-Prop $Item 'src'
  $dest    = Resolve-UserPath (Get-Prop $Item 'dest')
  $state   = Get-ItemState -Item $Item
  $include = Get-Prop $Item 'include'
  $exclude = Get-Prop $Item 'exclude'

  if (-not (Test-Path $dest)) { New-Item -ItemType Directory -Path $dest -Force | Out-Null }

  $cachePath = Get-ZipSha256CachePath -Item $Item
  $tempFile  = [System.IO.Path]::Combine([System.IO.Path]::GetTempPath(), [System.IO.Path]::GetRandomFileName() + '.zip')

  try {
    Write-Verbose "Downloading $src..."
    Invoke-WebRequest -Uri $src -OutFile $tempFile -UseBasicParsing

    $newHash = (Get-FileHash -Path $tempFile -Algorithm SHA256).Hash

    if ($state -eq 'latest' -and (Test-Path $cachePath)) {
      $cachedHash = (Get-Content -Path $cachePath -Raw).Trim()
      if ($cachedHash -eq $newHash) {
        Write-Verbose "SHA256 unchanged ($newHash); skipping extraction."
        return
      }
      Write-Verbose "SHA256 changed; re-extracting."
    }

    Add-Type -AssemblyName System.IO.Compression.FileSystem
    $zip = [System.IO.Compression.ZipFile]::OpenRead($tempFile)
    try {
      foreach ($entry in $zip.Entries) {
        if ($entry.Name -eq '') { continue }  # directory entry

        $name = $entry.Name

        if ($include -and @($include).Count -gt 0) {
          $matched = $false
          foreach ($pattern in @($include)) { if ($name -like $pattern) { $matched = $true; break } }
          if (-not $matched) { continue }
        }

        if ($exclude -and @($exclude).Count -gt 0) {
          $skip = $false
          foreach ($pattern in @($exclude)) { if ($name -like $pattern) { $skip = $true; break } }
          if ($skip) { continue }
        }

        $destPath = Join-Path $dest $name
        Write-Verbose "  Extracting: $name"
        [System.IO.Compression.ZipFileExtensions]::ExtractToFile($entry, $destPath, $true)
      }
    } finally {
      $zip.Dispose()
    }

    $cacheDir = Split-Path $cachePath -Parent
    if (-not (Test-Path $cacheDir)) { New-Item -ItemType Directory -Path $cacheDir -Force | Out-Null }
    Set-Content -Path $cachePath -Value $newHash -NoNewline
    Write-Verbose "Saved SHA256 to $cachePath"
  } finally {
    if (Test-Path $tempFile) { Remove-Item $tempFile -Force -ErrorAction SilentlyContinue }
  }
}

function Remove-ZipCreates {
  param($Item)
  Remove-CreatesPatterns -Creates (Get-Prop $Item 'creates')
  # Remove stale SHA256 cache so a future install re-downloads.
  $cachePath = Get-ZipSha256CachePath -Item $Item
  if (Test-Path $cachePath) { Remove-Item -Path $cachePath -Force -ErrorAction SilentlyContinue }
}

function Get-ZipHandler {
  [PSCustomObject]@{
    Test      = {
      param($Item)
      Test-CreatesPresent -Creates (Get-Prop $Item 'creates')
    }
    Describe  = {
      param($Item, $Action)
      $src  = Get-Prop $Item 'src'
      $dest = Resolve-UserPath (Get-Prop $Item 'dest')
      if ($Action -eq 'Uninstall') { "remove creates entries for $src -> $dest" }
      else { "download and extract $src -> $dest" }
    }
    Install   = {
      param($Item)
      Invoke-ZipDownloadAndExtract -Item $Item
    }
    Uninstall = {
      param($Item)
      Remove-ZipCreates -Item $Item
    }
  }
}

Export-ModuleMember -Function Get-ZipHandler
