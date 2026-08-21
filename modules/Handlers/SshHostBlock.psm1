#!/usr/bin/env pwsh
<#
.SYNOPSIS
  Handler for the 'ssh_host_block' module: renders one or more ssh_config
  "Host" blocks from structured data and writes them into a marker-delimited
  block, reusing BlockInFile.psm1's marker/insert/backup machinery.

.DESCRIPTION
  Each entry in 'hosts' is an arbitrary key/value map. Field names may be
  written as snake_case, camelCase, or PascalCase - normalized to the
  PascalCase OpenSSH expects (e.g. 'host_name'/'hostName'/'HostName' all
  become 'HostName'). Keys are never validated against real OpenSSH
  keywords - the caller is trusted to know what they're configuring.

  Two keys are reserved, never rendered as a directive:
    - 'host' (required): becomes the 'Host <value>' header line.
    - 'comment': rendered verbatim as a '# <comment>' line above the header.
  Every other scalar field becomes '<PascalCaseKey> <value>' (booleans render
  as yes/no). A list-valued field has no plural OpenSSH directive to render
  as (e.g. 'IdentityFile' has no 'IdentityFiles' form) - it's singularized
  (trailing 's' dropped from the PascalCased key) and repeated once per entry
  instead.

  'defaults' is merged under every entry (an entry's own keys win) - lets a
  set of hosts share common fields (e.g. 'user'/'identities_only') without
  repeating them per entry.

  'comment_template' auto-builds a 'comment' for any entry that doesn't set
  its own literal one, e.g. 'Enterprise: {host} (orgs: {orgs})' - '{key}'
  pulls that entry's own field (a list joins with ', '). Any key referenced
  this way is metadata for the comment, not a directive, so it's excluded
  from the per-key directive loop too (same treatment as 'host'/'comment').

  If an entry doesn't set its own host_name/hostName/HostName, one is
  auto-filled from 'host' - the common case (HostName mirrors Host) needs no
  extra typing, while an entry that wants something different just sets it.
#>

Set-StrictMode -Version Latest

Import-Module (Join-Path $PSScriptRoot '..\Common.psm1')
Import-Module (Join-Path $PSScriptRoot 'BlockInFile.psm1')

$script:DefaultMarker = '# {mark} IRONSTATE MANAGED - {name}'
# Hashtable/OrderedDictionary key lookups are case-insensitive, so only the
# underscore/no-underscore spellings need listing here - 'hostName' vs.
# 'HostName' vs. 'HOSTNAME' are already the same key as far as '.Contains()'
# is concerned.
$script:HostNameKeys = @('host_name', 'hostname')

function Convert-SshDirectiveKeyToPascalCase {
  param([string] $Name)
  # '-creplace' (case-sensitive), not '-replace' - PowerShell's '-replace' is
  # case-INsensitive by default, which would make '[a-z0-9]'/'[A-Z]' both
  # match either case and insert an underscore between nearly every letter.
  $normalized = $Name -creplace '([a-z0-9])([A-Z])', '$1_$2'
  $segments = $normalized -split '[_\s-]+' | Where-Object { $_ -ne '' }
  -join ($segments | ForEach-Object { $_.Substring(0, 1).ToUpper() + $_.Substring(1).ToLower() })
}

function ConvertTo-SshDirectiveValueString {
  param($Val)
  if ($Val -is [bool]) { return $(if ($Val) { 'yes' } else { 'no' }) }
  return [string] $Val
}

function ConvertTo-SshCommentFieldString {
  # Same scalar/bool formatting as a directive value, but a list joins with
  # ', ' instead of repeating - a comment is one line, not multiple.
  param($Val)
  if ($null -eq $Val) { return '' }
  if (($Val -is [System.Collections.IEnumerable]) -and ($Val -isnot [string])) {
    return ((@($Val) | ForEach-Object { [string] $_ }) -join ', ')
  }
  return ConvertTo-SshDirectiveValueString $Val
}

function Get-CommentTemplateKeys {
  # Every '{key}' placeholder referenced by a comment_template - these are
  # comment metadata, not directives, so the render loop below skips them.
  param([string] $Template)
  if (-not $Template) { return @() }
  return ,@([regex]::Matches($Template, '\{(\w+)\}') | ForEach-Object { $_.Groups[1].Value })
}

function Expand-SshCommentTemplate {
  param([string] $Template, [System.Collections.IDictionary] $Entry)
  $result = $Template
  foreach ($key in (Get-CommentTemplateKeys -Template $Template)) {
    $value = if ($Entry.Contains($key)) { ConvertTo-SshCommentFieldString $Entry[$key] } else { '' }
    $result = $result.Replace("{$key}", $value)
  }
  return $result
}

function Merge-SshHostEntry {
  # 'defaults' underneath the entry's own fields - entry keys win.
  param([System.Collections.IDictionary] $Entry, [System.Collections.IDictionary] $Defaults)
  $merged = [ordered]@{}
  foreach ($key in @($Defaults.Keys)) { $merged[$key] = $Defaults[$key] }
  foreach ($key in @($Entry.Keys)) { $merged[$key] = $Entry[$key] }
  return $merged
}

function Get-SshHostBlockEntryLines {
  param([System.Collections.IDictionary] $Entry, [string] $CommentTemplate)

  if (-not $Entry.Contains('host') -or -not $Entry['host']) {
    throw "'ssh_host_block' requires a 'host' key on every host entry"
  }

  $reservedKeys = @('host', 'comment') + (Get-CommentTemplateKeys -Template $CommentTemplate)
  $hasOwnHostName = @($script:HostNameKeys) | Where-Object { $Entry.Contains($_) }

  $lines = [System.Collections.Generic.List[string]]::new()

  $comment = if ($Entry.Contains('comment') -and $Entry['comment']) { [string] $Entry['comment'] }
    elseif ($CommentTemplate) { Expand-SshCommentTemplate -Template $CommentTemplate -Entry $Entry }
    else { $null }
  if ($comment) { $lines.Add("# $comment") }

  $lines.Add("Host $($Entry['host'])")
  if (-not $hasOwnHostName) { $lines.Add("  HostName $($Entry['host'])") }

  foreach ($key in @($Entry.Keys)) {
    if ($reservedKeys -contains $key) { continue }
    $val = $Entry[$key]
    if ($null -eq $val) { continue }

    if (($val -is [System.Collections.IEnumerable]) -and ($val -isnot [string])) {
      $directive = Convert-SshDirectiveKeyToPascalCase -Name $key
      if ($directive.EndsWith('s')) { $directive = $directive.Substring(0, $directive.Length - 1) }
      foreach ($item in @($val)) { $lines.Add("  $directive $(ConvertTo-SshDirectiveValueString $item)") }
    } else {
      $lines.Add("  $(Convert-SshDirectiveKeyToPascalCase -Name $key) $(ConvertTo-SshDirectiveValueString $val)")
    }
  }

  return ,$lines.ToArray()
}

function Get-SshHostBlockContent {
  param($Item)

  $defaults = Get-Prop $Item 'defaults' ([ordered]@{})
  $commentTemplate = Get-Prop $Item 'comment_template'
  $hosts = @(Get-Prop $Item 'hosts' @())

  $blocks = [System.Collections.Generic.List[string]]::new()
  foreach ($hostEntry in $hosts) {
    if ($null -eq $hostEntry) { continue }
    $entry = Merge-SshHostEntry -Entry $hostEntry -Defaults $defaults
    $blocks.Add((Get-SshHostBlockEntryLines -Entry $entry -CommentTemplate $commentTemplate) -join "`n")
  }

  return ($blocks -join "`n`n")
}

function Resolve-SshHostBlockIdentifier {
  param($Item, [string] $Name)
  Resolve-BlockIdentifier -Item $Item -Name $Name
}

function Test-SshHostBlockPresent {
  param($Item, [string] $Name)

  $dest = Resolve-UserPath (Get-Prop $Item 'dest')
  if (-not (Test-Path $dest)) { return $false }

  $markers = Get-BlockMarkers `
    -Marker (Get-Prop $Item 'marker' $script:DefaultMarker) `
    -MarkerBegin (Get-Prop $Item 'marker_begin' 'BEGIN') `
    -MarkerEnd (Get-Prop $Item 'marker_end' 'END') `
    -Name (Resolve-SshHostBlockIdentifier -Item $Item -Name $Name)

  $lines = @(Get-FileLines -Path $dest)
  $range = Find-BlockRange -Lines $lines -BeginMarker $markers.Begin -EndMarker $markers.End
  if (-not $range) { return $false }

  $existingBlockLines = if ($range.EndIndex -gt $range.BeginIndex + 1) {
    $lines[($range.BeginIndex + 1)..($range.EndIndex - 1)]
  } else { @() }

  $desiredBlockLines = @(Get-DesiredBlockLines -Block (Get-SshHostBlockContent -Item $Item))
  return (@($existingBlockLines) -join "`n") -eq (@($desiredBlockLines) -join "`n")
}

function Set-SshHostBlock {
  param($Item, [string] $Name)

  $dest = Resolve-UserPath (Get-Prop $Item 'dest')
  $create = [bool] (Get-Prop $Item 'create' $false)
  $exists = Test-Path $dest
  if (-not $exists -and -not $create) {
    Write-Warning "ssh_host_block dest does not exist and 'create' is false, skipping: $dest"
    return
  }

  $markers = Get-BlockMarkers `
    -Marker (Get-Prop $Item 'marker' $script:DefaultMarker) `
    -MarkerBegin (Get-Prop $Item 'marker_begin' 'BEGIN') `
    -MarkerEnd (Get-Prop $Item 'marker_end' 'END') `
    -Name (Resolve-SshHostBlockIdentifier -Item $Item -Name $Name)

  $lines = [System.Collections.Generic.List[string]]::new()
  if ($exists) { $lines.AddRange([string[]] @(Get-FileLines -Path $dest)) }

  [string[]] $newBlockLines = @($markers.Begin) + @(Get-DesiredBlockLines -Block (Get-SshHostBlockContent -Item $Item)) + @($markers.End)

  $range = Find-BlockRange -Lines $lines.ToArray() -BeginMarker $markers.Begin -EndMarker $markers.End

  if ($range) {
    $lines.RemoveRange($range.BeginIndex, $range.EndIndex - $range.BeginIndex + 1)
    $lines.InsertRange($range.BeginIndex, $newBlockLines)
  } else {
    $insertIndex = Get-BlockInsertIndex -Lines $lines.ToArray() `
      -InsertAfter (Get-Prop $Item 'insertafter' 'EOF') `
      -InsertBefore (Get-Prop $Item 'insertbefore')
    $lines.InsertRange($insertIndex, $newBlockLines)
  }

  if ($exists -and [bool] (Get-Prop $Item 'backup' $false)) { Backup-BlockInFileDest -Path $dest }

  $destDir = Split-Path $dest -Parent
  if ($destDir -and -not (Test-Path $destDir)) { New-Item -ItemType Directory -Path $destDir -Force | Out-Null }

  Write-BlockInFileLines -Path $dest -Lines $lines
}

function Remove-SshHostBlock {
  param($Item, [string] $Name)

  $dest = Resolve-UserPath (Get-Prop $Item 'dest')
  if (-not (Test-Path $dest)) { return }

  $markers = Get-BlockMarkers `
    -Marker (Get-Prop $Item 'marker' $script:DefaultMarker) `
    -MarkerBegin (Get-Prop $Item 'marker_begin' 'BEGIN') `
    -MarkerEnd (Get-Prop $Item 'marker_end' 'END') `
    -Name (Resolve-SshHostBlockIdentifier -Item $Item -Name $Name)

  $lines = [System.Collections.Generic.List[string]]::new()
  $lines.AddRange([string[]] @(Get-FileLines -Path $dest))

  $range = Find-BlockRange -Lines $lines.ToArray() -BeginMarker $markers.Begin -EndMarker $markers.End
  if (-not $range) { return }

  if ([bool] (Get-Prop $Item 'backup' $false)) { Backup-BlockInFileDest -Path $dest }

  $lines.RemoveRange($range.BeginIndex, $range.EndIndex - $range.BeginIndex + 1)
  Write-BlockInFileLines -Path $dest -Lines $lines
}

function Get-SshHostBlockHandler {
  [PSCustomObject]@{
    Test      = {
      param($Item, $Name)
      Test-SshHostBlockPresent -Item $Item -Name $Name
    }
    Describe  = {
      param($Item, $Action)
      $dest = Resolve-UserPath (Get-Prop $Item 'dest')
      if ($Action -eq 'Uninstall') { "remove ironstate managed ssh host block from $dest" } else { "manage ssh host block in $dest" }
    }
    Install   = {
      param($Item, $Name)
      Set-SshHostBlock -Item $Item -Name $Name
    }
    Uninstall = {
      param($Item, $Name)
      Remove-SshHostBlock -Item $Item -Name $Name
    }
  }
}

Export-ModuleMember -Function Get-SshHostBlockHandler
