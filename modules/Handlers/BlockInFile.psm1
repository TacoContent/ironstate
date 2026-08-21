#!/usr/bin/env pwsh
<#
.SYNOPSIS
  Handler for the 'blockinfile' group: inserts/updates/removes a
  marker-delimited block of text in a file, modeled on Ansible's
  ansible.builtin.blockinfile.

.DESCRIPTION
  A block is wrapped in a pair of marker lines built from 'marker' (default
  '# {mark} IRONSTATE MANAGED - {name}') with '{mark}' replaced by
  'marker_begin'/'marker_end' (default BEGIN/END) and '{name}' replaced by
  an identifier - 'marker_name' if set, else the task's own 'name', else
  'dest's file name - so multiple blockinfile tasks can share the same
  'dest' without one overwriting another's block. Only the text between
  those exact marker lines is ever touched - everything else in the file is
  left alone. A custom 'marker' template with no '{name}' token is left as
  a fixed, unlabeled marker - '{name}' substitution is a no-op when the
  token isn't present, matching how '{mark}' already behaves.

  If the markers already exist in the file, the block between them is
  replaced in place. Otherwise the new block is inserted per 'insertafter'/
  'insertbefore' (default: end of file). 'create' controls whether a missing
  'dest' is created (default false, matching Ansible); 'backup' writes a
  timestamped copy of 'dest' before it's changed.

  "installed" (used by the present/absent/latest state machine in
  Common.psm1) means the marker block exists AND its content already
  matches 'block' (or the rendered 'template', see below) exactly - the
  same "exact match" convention Copy.psm1 and Symlinks.psm1 use for their
  own Test.

  'template' is a 'src'/'engine'/'vars' object (same shape as the
  'template' module's own fields, minus 'dest'/'state' - this handler
  already owns those via 'dest'/'state') - when given instead of 'block',
  the block's content is Handlers/Template.psm1's
  Get-TemplateRenderedContent render of that template, rather than a
  literal string. Mutually exclusive with 'block' in practice (not
  schema-enforced, matching this codebase's existing convention for e.g.
  'shell's 'command'/'script') - 'template' wins if both are given.
#>

Set-StrictMode -Version Latest

Import-Module (Join-Path $PSScriptRoot '..\Common.psm1')
Import-Module (Join-Path $PSScriptRoot 'Template.psm1')

function Get-BlockInFileContent {
  # Resolves this task's desired block content: a rendered template if
  # 'template' is set, else the literal 'block' string (default '').
  param($Item, $Context)
  $templateSpec = Get-Prop $Item 'template'
  if ($templateSpec) {
    $rendered = Get-TemplateRenderedContent -Item $templateSpec -Context $Context
    if ($null -ne $rendered) { return $rendered }
    return ''
  }
  return Get-Prop $Item 'block' ''
}

$script:DefaultMarker = '# {mark} IRONSTATE MANAGED - {name}'

function Get-BlockMarkers {
  param([string] $Marker, [string] $MarkerBegin, [string] $MarkerEnd, [string] $Name)
  [PSCustomObject]@{
    Begin = $Marker.Replace('{mark}', $MarkerBegin).Replace('{name}', $Name)
    End   = $Marker.Replace('{mark}', $MarkerEnd).Replace('{name}', $Name)
  }
}

function Resolve-BlockIdentifier {
  # 'marker_name' wins if set, else the task's own display name, else
  # 'dest's file name - always non-empty since 'dest' is required, so
  # '{name}' in the default marker always resolves to something distinct
  # per task even when neither 'marker_name' nor the task's 'name' is set.
  param($Item, [string] $Name)
  $override = Get-Prop $Item 'marker_name'
  if ($override) { return $override }
  if ($Name) { return $Name }
  $dest = Get-Prop $Item 'dest'
  if ($dest) { return (Split-Path (Resolve-UserPath $dest) -Leaf) }
  return ''
}

function Get-FileLines {
  # Splits a file's content into lines, normalizing CRLF -> LF first.
  # Returns @() for a missing or empty file (including a 'dest' that turns
  # out to be a directory - Test-Path is true for those too, but reading one
  # as a file yields no content). Deliberately returns a *plain* array, with
  # no leading-comma "protect the 0/1-element case across the return
  # boundary" trick - every call site below wraps the call in '@(...)'
  # itself instead, which already guarantees an array of the right shape
  # for 0/1/N elements. Mixing the two conventions is what caused a real
  # bug: '@(Get-X)' around a comma-protected return double-wraps a multi-
  # element result into a single array *element*, and casting that to
  # '[string[]]' silently ToString()-joins it into one line (see
  # Set-BlockInFile's history) - so don't reintroduce the leading comma
  # here without also dropping every caller's '@()' wrap, or vice versa.
  param([string] $Path)
  if (-not (Test-Path $Path)) { return @() }
  $raw = Get-Content -Path $Path -Raw -ErrorAction SilentlyContinue
  if ([string]::IsNullOrEmpty($raw)) { return @() }
  return ($raw -replace "`r`n", "`n") -split "`n"
}

function Get-DesiredBlockLines {
  # Same plain-array convention as Get-FileLines above - see its comment.
  param([string] $Block)
  if ([string]::IsNullOrEmpty($Block)) { return @() }
  $normalized = ($Block -replace "`r`n", "`n").TrimEnd("`n")
  return $normalized -split "`n"
}

function Find-BlockRange {
  # Locates the begin/end marker lines (exact match, trailing whitespace
  # ignored). Both markers must be present, end after begin, or the block
  # is treated as absent.
  param([string[]] $Lines, [string] $BeginMarker, [string] $EndMarker)

  $beginIndex = -1
  for ($i = 0; $i -lt $Lines.Count; $i++) {
    if ($Lines[$i].TrimEnd() -eq $BeginMarker) { $beginIndex = $i; break }
  }
  if ($beginIndex -lt 0) { return $null }

  $endIndex = -1
  for ($i = $beginIndex + 1; $i -lt $Lines.Count; $i++) {
    if ($Lines[$i].TrimEnd() -eq $EndMarker) { $endIndex = $i; break }
  }
  if ($endIndex -lt 0) { return $null }

  [PSCustomObject]@{ BeginIndex = $beginIndex; EndIndex = $endIndex }
}

function Get-BlockInsertIndex {
  # 'insertbefore' wins if both are given. 'BOF'/'EOF' are literal
  # positions; anything else is a regex matched against existing lines -
  # insertbefore uses the first match, insertafter the last (closest to
  # Ansible's own behavior). A regex that matches nothing falls back to EOF.
  param([string[]] $Lines, [string] $InsertAfter, [string] $InsertBefore)

  if ($InsertBefore) {
    if ($InsertBefore -eq 'BOF') { return 0 }
    for ($i = 0; $i -lt $Lines.Count; $i++) { if ($Lines[$i] -match $InsertBefore) { return $i } }
    return $Lines.Count
  }

  if ($InsertAfter -and $InsertAfter -ne 'EOF') {
    if ($InsertAfter -eq 'BOF') { return 0 }
    $matchIndex = -1
    for ($i = 0; $i -lt $Lines.Count; $i++) { if ($Lines[$i] -match $InsertAfter) { $matchIndex = $i } }
    if ($matchIndex -ge 0) { return $matchIndex + 1 }
    return $Lines.Count
  }

  return $Lines.Count
}

function Write-BlockInFileLines {
  # Joins lines back with LF, ensures a trailing newline, and writes them.
  param([string] $Path, [System.Collections.Generic.List[string]] $Lines)
  $content = ($Lines -join "`n")
  if ($content -ne '' -and -not $content.EndsWith("`n")) { $content += "`n" }
  Set-Content -Path $Path -Value $content -NoNewline
}

function Backup-BlockInFileDest {
  param([string] $Path)
  $backupPath = "$Path.$(Get-Date -Format 'yyyyMMddHHmmss').bak"
  Copy-Item -Path $Path -Destination $backupPath -Force
}

function Test-BlockInFilePresent {
  param($Item, [string] $Name, $Context)

  $dest = Resolve-UserPath (Get-Prop $Item 'dest')
  if (-not (Test-Path $dest)) { return $false }

  $markers = Get-BlockMarkers `
    -Marker (Get-Prop $Item 'marker' $script:DefaultMarker) `
    -MarkerBegin (Get-Prop $Item 'marker_begin' 'BEGIN') `
    -MarkerEnd (Get-Prop $Item 'marker_end' 'END') `
    -Name (Resolve-BlockIdentifier -Item $Item -Name $Name)

  $lines = @(Get-FileLines -Path $dest)
  $range = Find-BlockRange -Lines $lines -BeginMarker $markers.Begin -EndMarker $markers.End
  if (-not $range) { return $false }

  $existingBlockLines = if ($range.EndIndex -gt $range.BeginIndex + 1) {
    $lines[($range.BeginIndex + 1)..($range.EndIndex - 1)]
  } else { @() }

  $desiredBlockLines = @(Get-DesiredBlockLines -Block (Get-BlockInFileContent -Item $Item -Context $Context))
  return (@($existingBlockLines) -join "`n") -eq (@($desiredBlockLines) -join "`n")
}

function Set-BlockInFile {
  param($Item, [string] $Name, $Context)

  $dest = Resolve-UserPath (Get-Prop $Item 'dest')
  $create = [bool] (Get-Prop $Item 'create' $false)
  $exists = Test-Path $dest
  if (-not $exists -and -not $create) {
    Write-Warning "blockinfile dest does not exist and 'create' is false, skipping: $dest"
    return
  }

  $markers = Get-BlockMarkers `
    -Marker (Get-Prop $Item 'marker' $script:DefaultMarker) `
    -MarkerBegin (Get-Prop $Item 'marker_begin' 'BEGIN') `
    -MarkerEnd (Get-Prop $Item 'marker_end' 'END') `
    -Name (Resolve-BlockIdentifier -Item $Item -Name $Name)

  $lines = [System.Collections.Generic.List[string]]::new()
  if ($exists) { $lines.AddRange([string[]] @(Get-FileLines -Path $dest)) }

  [string[]] $newBlockLines = @($markers.Begin) + @(Get-DesiredBlockLines -Block (Get-BlockInFileContent -Item $Item -Context $Context)) + @($markers.End)

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

function Remove-BlockInFile {
  param($Item, [string] $Name)

  $dest = Resolve-UserPath (Get-Prop $Item 'dest')
  if (-not (Test-Path $dest)) { return }

  $markers = Get-BlockMarkers `
    -Marker (Get-Prop $Item 'marker' $script:DefaultMarker) `
    -MarkerBegin (Get-Prop $Item 'marker_begin' 'BEGIN') `
    -MarkerEnd (Get-Prop $Item 'marker_end' 'END') `
    -Name (Resolve-BlockIdentifier -Item $Item -Name $Name)

  $lines = [System.Collections.Generic.List[string]]::new()
  $lines.AddRange([string[]] @(Get-FileLines -Path $dest))

  $range = Find-BlockRange -Lines $lines.ToArray() -BeginMarker $markers.Begin -EndMarker $markers.End
  if (-not $range) { return }

  if ([bool] (Get-Prop $Item 'backup' $false)) { Backup-BlockInFileDest -Path $dest }

  $lines.RemoveRange($range.BeginIndex, $range.EndIndex - $range.BeginIndex + 1)
  Write-BlockInFileLines -Path $dest -Lines $lines
}

function Get-BlockInFileHandler {
  [PSCustomObject]@{
    Test      = {
      param($Item, $Name, $Context)
      Test-BlockInFilePresent -Item $Item -Name $Name -Context $Context
    }
    Describe  = {
      param($Item, $Action, $Context)
      $dest = Resolve-UserPath (Get-Prop $Item 'dest')
      if ($Action -eq 'Uninstall') { "remove ironstate managed block from $dest" } else { "manage block in $dest" }
    }
    Install   = {
      param($Item, $Name, $Context)
      Set-BlockInFile -Item $Item -Name $Name -Context $Context
    }
    Uninstall = {
      param($Item, $Name)
      Remove-BlockInFile -Item $Item -Name $Name
    }
  }
}

Export-ModuleMember -Function Get-BlockInFileHandler, Get-BlockMarkers, Resolve-BlockIdentifier, Get-FileLines, Get-DesiredBlockLines, Find-BlockRange, Get-BlockInsertIndex, Write-BlockInFileLines, Backup-BlockInFileDest
