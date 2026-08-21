#!/usr/bin/env pwsh
<#
.SYNOPSIS
  Handler for the 'file' group: manages a path as a plain file, directory,
  symlink, or hard link - modeled on Ansible's ansible.builtin.file.

.DESCRIPTION
  { path: <string>, type: file|directory|link|hard|touch (default 'file'),
    src: <string> (required for type 'link'/'hard' - the existing path the
    link points to), force: <bool> (default false), state: present|absent|
    latest }.

  'state' here is this codebase's usual present/absent/latest state
  machine (Common.psm1), same as every other handler - NOT Ansible's own
  'file' module, which overloads its 'state' field to also mean the target
  *kind* (file/directory/link/hard/touch) plus 'absent'. Doing that here
  would break the generic Get-ItemState/Resolve-PackageAction dispatch
  every handler shares, since it only understands present/absent/latest -
  so what kind of thing to manage is the separate 'type' field instead.

  'present'/'latest' ensure 'path' exists as 'type' (creating parent
  directories as needed); 'absent' removes whatever is at 'path' -
  recursively for a real directory, but a link/hard link is only ever
  unlinked (Remove-Item without '-Recurse'), never recursed into - so
  removing a directory symlink can't ever delete the target's contents.

  Unlike Ansible, 'type: file' creates an empty file if 'path' is missing
  rather than erroring (only Ansible's 'touch' state does that) - a bare
  assert-only mode isn't useful here, since every other handler in this
  codebase treats "missing" as "make it so" for 'present'/'latest'.
  'type: touch' is still available for the Unix-'touch' behavior of always
  updating the timestamp (it never reports "already installed").

  'force' governs what happens when 'path' already exists as something
  *other* than the requested 'type' (e.g. a real file where a symlink was
  wanted): false (default) warns and skips rather than destroying whatever
  is already there; true replaces it.
#>

Set-StrictMode -Version Latest

Import-Module (Join-Path $PSScriptRoot '..\Common.psm1')

function Get-FilePathKind {
  # Classifies what already exists at $Path: 'missing', 'link' (symlink),
  # 'hard' (hard link), 'directory', or 'file' - checked in this order
  # since a reparse point pointing at a directory still looks like a
  # container to Test-Path, so LinkType has to be checked first.
  param([string] $Path)
  if (-not (Test-Path $Path)) { return 'missing' }
  $item = Get-Item -Path $Path -Force
  if ($item.LinkType -eq 'SymbolicLink') { return 'link' }
  if ($item.LinkType -eq 'HardLink') { return 'hard' }
  if ($item -is [System.IO.DirectoryInfo]) { return 'directory' }
  return 'file'
}

function Remove-FileItemAtPath {
  # Clears whatever is at $Path so a different 'type' can be created in its
  # place. Recurses only for a real directory - a link/hard link is
  # unlinked without ever touching what it points to.
  param([string] $Path, [string] $Kind)
  if ($Kind -eq 'directory') { Remove-Item -Path $Path -Recurse -Force }
  else { Remove-Item -Path $Path -Force }
}

function Test-FileItemPresent {
  param($Item)
  $path = Resolve-UserPath (Get-Prop $Item 'path')
  $type = Get-Prop $Item 'type' 'file'

  if ($type -eq 'touch') { return $false } # always fires, like Log/Fact

  $kind = Get-FilePathKind -Path $path
  switch ($type) {
    'directory' { return $kind -eq 'directory' }
    'link' {
      if ($kind -ne 'link') { return $false }
      $src = ConvertTo-NormalizedPathString -Path (Resolve-UserPath (Get-Prop $Item 'src'))
      $targets = @((Get-Item -Path $path -Force).Target | ForEach-Object { ConvertTo-NormalizedPathString -Path $_ })
      return $targets -contains $src
    }
    'hard' {
      if ($kind -ne 'hard') { return $false }
      $src = Resolve-UserPath (Get-Prop $Item 'src')
      if (-not (Test-Path $src)) { return $false }
      return (Get-FileHash -Path $path -Algorithm SHA256).Hash -eq (Get-FileHash -Path $src -Algorithm SHA256).Hash
    }
    default { return $kind -eq 'file' } # 'file'
  }
}

function Install-FileItem {
  param($Item)
  $path = Resolve-UserPath (Get-Prop $Item 'path')
  $type = Get-Prop $Item 'type' 'file'
  $force = [bool] (Get-Prop $Item 'force' $false)

  $parent = Split-Path $path -Parent
  if ($parent -and -not (Test-Path $parent)) { New-Item -ItemType Directory -Path $parent -Force | Out-Null }

  if ($type -eq 'touch') {
    if (-not (Test-Path $path)) { New-Item -ItemType File -Path $path -Force | Out-Null }
    else { (Get-Item -Path $path -Force).LastWriteTime = Get-Date }
    return
  }

  if ($type -eq 'link' -or $type -eq 'hard') {
    $src = Resolve-UserPath (Get-Prop $Item 'src')
    if (-not (Test-Path $src)) { Write-Warning "Source path for '$type' does not exist, skipping: $src"; return }
  }

  if (Test-FileItemPresent -Item $Item) { return }

  $kind = Get-FilePathKind -Path $path
  if ($kind -ne 'missing') {
    if (-not $force) { Write-Warning "path already exists as something else, skipping (set force: true to replace): $path"; return }
    Remove-FileItemAtPath -Path $path -Kind $kind
  }

  switch ($type) {
    'directory' { New-Item -ItemType Directory -Path $path -Force | Out-Null }
    'link'      { New-Item -ItemType SymbolicLink -Path $path -Target $src -Force | Out-Null }
    'hard'      { New-Item -ItemType HardLink -Path $path -Target $src -Force | Out-Null }
    default     { New-Item -ItemType File -Path $path -Force | Out-Null }
  }
}

function Uninstall-FileItem {
  param($Item)
  $path = Resolve-UserPath (Get-Prop $Item 'path')
  if (-not (Test-Path $path)) { return }
  Remove-FileItemAtPath -Path $path -Kind (Get-FilePathKind -Path $path)
}

function Get-FileHandler {
  [PSCustomObject]@{
    Test      = {
      param($Item)
      Test-FileItemPresent -Item $Item
    }
    Describe  = {
      param($Item, $Action)
      $path = Resolve-UserPath (Get-Prop $Item 'path')
      $type = Get-Prop $Item 'type' 'file'
      if ($Action -eq 'Uninstall') { "remove $path" }
      elseif ($type -eq 'link') { "link $path -> $(Get-Prop $Item 'src')" }
      elseif ($type -eq 'hard') { "hard link $path -> $(Get-Prop $Item 'src')" }
      else { "ensure $type $path" }
    }
    Install   = {
      param($Item)
      Install-FileItem -Item $Item
    }
    Uninstall = {
      param($Item)
      Uninstall-FileItem -Item $Item
    }
  }
}

Export-ModuleMember -Function Get-FileHandler, Test-FileItemPresent, Install-FileItem, Uninstall-FileItem
