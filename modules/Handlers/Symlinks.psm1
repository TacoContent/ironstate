#!/usr/bin/env pwsh
<#
.SYNOPSIS
  Handler for the 'symlinks' group: a symbolic link from 'src' to 'dest'.

.DESCRIPTION
  Thin wrapper over File.psm1's 'link' type - translates 'src'/'dest' into
  a { path; type: link; src; force } item and delegates every Test/Install/
  Uninstall to it, rather than duplicating symlink-creation logic.

  'force' defaults to true here (unlike File.psm1's own default of false),
  preserving this handler's original behavior of always replacing whatever
  was already at 'dest' - set 'force: false' to opt into File.psm1's safer
  warn-and-skip behavior instead.
#>

Set-StrictMode -Version Latest

Import-Module (Join-Path $PSScriptRoot '..\Common.psm1')
Import-Module (Join-Path $PSScriptRoot 'File.psm1')

function ConvertTo-FileLinkItem {
  param($Item)
  @{
    path  = Get-Prop $Item 'dest'
    type  = 'link'
    src   = Get-Prop $Item 'src'
    force = [bool] (Get-Prop $Item 'force' $true)
  }
}

function Get-SymlinksHandler {
  [PSCustomObject]@{
    Test      = {
      param($Item)
      Test-FileItemPresent -Item (ConvertTo-FileLinkItem -Item $Item)
    }
    Describe  = {
      param($Item, $Action)
      $dest = Resolve-UserPath (Get-Prop $Item 'dest')
      $src = Resolve-UserPath (Get-Prop $Item 'src')
      if ($Action -eq 'Uninstall') { "remove symlink $dest" } else { "link $dest -> $src" }
    }
    Install   = {
      param($Item)
      Install-FileItem -Item (ConvertTo-FileLinkItem -Item $Item)
    }
    Uninstall = {
      param($Item)
      Uninstall-FileItem -Item (ConvertTo-FileLinkItem -Item $Item)
    }
  }
}

Export-ModuleMember -Function Get-SymlinksHandler
