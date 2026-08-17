#!/usr/bin/env pwsh
<#
.SYNOPSIS
  Handler for the 'registry' module: writes one or more named values under a
  single registry key.

.DESCRIPTION
  { path: <string>, values: [ { name, type, value }, ... ], state }. 'path'
  supports the usual hive shortcuts - HKLM, HKCU, HKCR, HKU, HKCC - and their
  full HKEY_* names (e.g. 'HKEY_LOCAL_MACHINE\Software\...'), with or without
  a trailing ':' or forward slashes; HKCR/HKU/HKCC aren't mounted as PSDrives
  by default, so this mounts them (once, at Global scope) on first use.

  'type' is one of String, ExpandString, Binary, DWord, MultiString, QWord
  (matched case-insensitively, so 'DWORD'/'QWORD' work too) - the same set
  New-ItemProperty's own '-PropertyType' accepts. 'value' matches the type:
  a scalar for String/ExpandString/DWord/QWord, a list for MultiString
  (strings) or Binary (byte values 0-255).

  Test is state-aware (like Log.psm1's trick, but branching on content
  rather than reusing a single boolean both ways): for 'present'/'latest' it
  requires every named value to exist with the exact type and data (so a
  mismatched value gets corrected, not skipped); for 'absent' it only needs
  *any* of the named values to still exist, regardless of correctness (so a
  stale wrong-typed value still gets removed). 'absent' removes only the
  named values, never the key itself.
#>

Set-StrictMode -Version Latest

Import-Module (Join-Path $PSScriptRoot '..\Common.psm1')

$script:ValidTypes = @('String', 'ExpandString', 'Binary', 'DWord', 'MultiString', 'QWord')

# Hive shortcut/full-name -> canonical PSDrive name, and that drive's
# '-Root' for New-PSDrive (HKLM:/HKCU: are mounted by PowerShell itself by
# default; HKCR:/HKU:/HKCC: are not, so those get mounted on first use).
$script:HiveAliases = @{
  HKLM = 'HKLM'; HKEY_LOCAL_MACHINE = 'HKLM'
  HKCU = 'HKCU'; HKEY_CURRENT_USER = 'HKCU'
  HKCR = 'HKCR'; HKEY_CLASSES_ROOT = 'HKCR'
  HKU  = 'HKU';  HKEY_USERS = 'HKU'
  HKCC = 'HKCC'; HKEY_CURRENT_CONFIG = 'HKCC'
}
$script:HiveRoots = @{
  HKLM = 'HKEY_LOCAL_MACHINE'
  HKCU = 'HKEY_CURRENT_USER'
  HKCR = 'HKEY_CLASSES_ROOT'
  HKU  = 'HKEY_USERS'
  HKCC = 'HKEY_CURRENT_CONFIG'
}

function Resolve-RegistryPath {
  param([Parameter(Mandatory)][string] $Path)

  $normalized = ($Path -replace '/', '\').TrimStart('\')
  $splitIndex = $normalized.IndexOf('\')
  $hiveToken  = if ($splitIndex -ge 0) { $normalized.Substring(0, $splitIndex) } else { $normalized }
  $rest       = if ($splitIndex -ge 0) { $normalized.Substring($splitIndex + 1) } else { '' }
  $hiveToken  = $hiveToken.TrimEnd(':').ToUpperInvariant()

  if (-not $script:HiveAliases.Contains($hiveToken)) {
    throw "Unknown registry hive '$hiveToken' in path '$Path' (expected one of: HKLM, HKCU, HKCR, HKU, HKCC, or their HKEY_* full names)"
  }
  $hive = $script:HiveAliases[$hiveToken]

  if (-not (Get-PSDrive -Name $hive -ErrorAction SilentlyContinue)) {
    New-PSDrive -Name $hive -PSProvider Registry -Root $script:HiveRoots[$hive] -Scope Global | Out-Null
  }

  if ($rest) { return "${hive}:\$rest" }
  return "${hive}:"
}

function ConvertTo-RegistryValueType {
  param([Parameter(Mandatory)][string] $Type)
  $matched = $script:ValidTypes | Where-Object { $_ -ieq $Type }
  if (-not $matched) { throw "Unknown registry value type '$Type' (expected one of: $($script:ValidTypes -join ', '))" }
  return $matched
}

function ConvertTo-RegistryData {
  param([Parameter(Mandatory)][string] $Type, $Value)
  switch ($Type) {
    'DWord' { return [int32] $Value }
    'QWord' { return [int64] $Value }
    'Binary' { return [byte[]] @(@($Value) | ForEach-Object { [byte] $_ }) }
    'MultiString' { return [string[]] @($Value) }
    default { return [string] $Value }
  }
}

function Test-RegistryValueEqual {
  param($Current, $Desired, [Parameter(Mandatory)][string] $Type)
  switch ($Type) {
    { $_ -in @('Binary', 'MultiString') } {
      $a = @($Current); $b = @($Desired)
      if ($a.Count -ne $b.Count) { return $false }
      for ($i = 0; $i -lt $a.Count; $i++) { if ("$($a[$i])" -cne "$($b[$i])") { return $false } }
      return $true
    }
    { $_ -in @('DWord', 'QWord') } { return [int64] $Current -eq [int64] $Desired }
    default { return [string] $Current -ceq [string] $Desired }
  }
}

function Get-RegistryValueSpecs {
  param($Item)
  return @(Get-Prop $Item 'values' @())
}

function Get-RawRegistryValue {
  # Get-ItemProperty auto-*expands* ExpandString (REG_EXPAND_SZ) values on
  # read - comparing that against the raw desired string would always
  # mismatch, so idempotency checks read straight off the underlying
  # RegistryKey with DoNotExpandEnvironmentNames instead. Returns $null when
  # the name doesn't exist (a real stored $null is indistinguishable from
  # "missing" here, which is an acceptable edge case for this purpose).
  param([Parameter(Mandatory)] $KeyItem, [Parameter(Mandatory)][string] $Name)
  return $KeyItem.GetValue($Name, $null, [Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames)
}

function Get-RegistryHandler {
  [PSCustomObject]@{
    Test      = {
      param($Item)
      $keyPath = Resolve-RegistryPath -Path (Get-Prop $Item 'path')
      $specs   = Get-RegistryValueSpecs -Item $Item

      if (-not (Test-Path $keyPath)) { return $false }
      $keyItem = Get-Item -Path $keyPath

      if ((Get-ItemState -Item $Item) -eq 'absent') {
        foreach ($spec in $specs) {
          if ($null -ne (Get-RawRegistryValue -KeyItem $keyItem -Name (Get-Prop $spec 'name'))) { return $true }
        }
        return $false
      }

      foreach ($spec in $specs) {
        $name = Get-Prop $spec 'name'
        $type = ConvertTo-RegistryValueType -Type (Get-Prop $spec 'type')
        $existing = Get-RawRegistryValue -KeyItem $keyItem -Name $name
        if ($null -eq $existing) { return $false }
        if ($keyItem.GetValueKind($name).ToString() -ne $type) { return $false }
        if (-not (Test-RegistryValueEqual -Current $existing -Desired (Get-Prop $spec 'value') -Type $type)) { return $false }
      }
      return $true
    }
    Describe  = {
      param($Item, $Action)
      $keyPath = Get-Prop $Item 'path'
      $names = (Get-RegistryValueSpecs -Item $Item | ForEach-Object { Get-Prop $_ 'name' }) -join ', '
      if ($Action -eq 'Uninstall') { "remove registry value(s) [$names] under $keyPath" } else { "set registry value(s) [$names] under $keyPath" }
    }
    Install   = {
      param($Item)
      $keyPath = Resolve-RegistryPath -Path (Get-Prop $Item 'path')
      if (-not (Test-Path $keyPath)) { New-Item -Path $keyPath -Force | Out-Null }
      foreach ($spec in Get-RegistryValueSpecs -Item $Item) {
        $type = ConvertTo-RegistryValueType -Type (Get-Prop $spec 'type')
        $data = ConvertTo-RegistryData -Type $type -Value (Get-Prop $spec 'value')
        New-ItemProperty -Path $keyPath -Name (Get-Prop $spec 'name') -PropertyType $type -Value $data -Force | Out-Null
      }
    }
    Uninstall = {
      param($Item)
      $keyPath = Resolve-RegistryPath -Path (Get-Prop $Item 'path')
      if (-not (Test-Path $keyPath)) { return }
      foreach ($spec in Get-RegistryValueSpecs -Item $Item) {
        Remove-ItemProperty -Path $keyPath -Name (Get-Prop $spec 'name') -Force -ErrorAction SilentlyContinue
      }
    }
  }
}

Export-ModuleMember -Function Get-RegistryHandler
