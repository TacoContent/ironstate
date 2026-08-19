#!/usr/bin/env pwsh
<#
.SYNOPSIS
  Handler for the 'path' module: ensures directories are present on (or
  absent from) the current user's persistent PATH environment variable.

.DESCRIPTION
  Reads/writes the User-scope PATH (not Machine-scope - no admin required,
  consistent with every other module in this system installing under
  '~/.local/bin' etc.). Also patches the *current* process's $env:PATH for
  entries it actually adds/removes, so later steps in the same run (e.g. an
  'eget' binary just installed) are immediately on PATH without a new shell.
#>

Set-StrictMode -Version Latest

Import-Module (Join-Path $PSScriptRoot '..\Common.psm1')

function Get-UserPathEntries {
  $current = [Environment]::GetEnvironmentVariable('PATH', 'User')
  if ([string]::IsNullOrEmpty($current)) { return ,@() }
  return ,@($current -split ';' | Where-Object { $_ })
}

function Get-WantedPaths {
  # Resolving '~' via a '| ForEach-Object' pipeline and assigning straight to
  # a variable collapses to a bare string when 'paths' has exactly one entry
  # (PowerShell unwraps single-item pipeline output) - wrapping the whole
  # pipeline expression in '@()' is what prevents that, not wrapping the
  # input alone.
  param($Item)
  return ,@(@(Get-Prop $Item 'paths' @()) | ForEach-Object { Resolve-UserPath $_ })
}

function Get-PathHandler {
  [PSCustomObject]@{
    Test      = {
      param($Item)
      $wanted = Get-WantedPaths -Item $Item
      if ($wanted.Count -eq 0) { return $true }
      $current = Get-UserPathEntries
      foreach ($p in $wanted) {
        if ($current -notcontains $p) { return $false }
      }
      return $true
    }
    Describe  = {
      param($Item, $Action)
      $paths = (Get-WantedPaths -Item $Item) -join ', '
      $scope = Get-Prop $Item 'scope' 'User'
      if ($Action -eq 'Uninstall') { "remove from PATH: $paths (scope: $scope)" } else { "add to PATH: $paths (scope: $scope)" }
    }
    Install   = {
      param($Item)
      $wanted = Get-WantedPaths -Item $Item
      $scope = Get-Prop $Item 'scope' 'User'
      $updated = [System.Collections.Generic.List[string]]::new()
      foreach ($p in (Get-UserPathEntries)) { $updated.Add($p) }

      $added = [System.Collections.Generic.List[string]]::new()
      foreach ($p in $wanted) {
        if ($updated -notcontains $p) { $updated.Add($p); $added.Add($p) }
      }

      [Environment]::SetEnvironmentVariable('PATH', ($updated -join ';'), $scope)
      foreach ($p in $added) { $env:PATH = "$env:PATH;$p" }
    }
    Uninstall = {
      param($Item)
      $unwanted = Get-WantedPaths -Item $Item
      $scope = Get-Prop $Item 'scope' 'User'

      $updated = @(Get-UserPathEntries | Where-Object { $unwanted -notcontains $_ })
      [Environment]::SetEnvironmentVariable('PATH', ($updated -join ';'), $scope)

      $liveUpdated = @($env:PATH -split ';' | Where-Object { $unwanted -notcontains $_ })
      $env:PATH = ($liveUpdated -join ';')
    }
  }
}

Export-ModuleMember -Function Get-PathHandler
