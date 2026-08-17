#!/usr/bin/env pwsh
<#
.SYNOPSIS
  '${{ ... }}' template expression expansion for modular packages.

.DESCRIPTION
  Modeled after GitHub Actions expressions: a package's main.yml can
  reference '${{ inputs.<key> }}' (from the package reference's 'with:'
  block) and '${{ package.name }}' / '${{ package.state }}' /
  '${{ package.tags }}' (from the package reference itself). Nothing is
  applied automatically - a package author opts in by writing the
  expression wherever they want the value, including as the entire value
  of a field (e.g. 'state: ${{ package.state }}', 'tags: ${{ package.tags }}')
  to receive that field's native type (string, array, ...) rather than a
  stringified copy. Paths may also index into a list with '[N]', e.g.
  '${{ example_task.results[0].rc }}'.

  The text inside '${{ ... }}' is a full expression, not just a bare path -
  see Expressions.psm1 (shared with the 'when:' grammar in
  Conditions.psm1) for the grammar. In particular, a value can be piped
  through filters: '${{ languages.java.jdk | default("Oracle.JDK.25") }}'
  falls back to the literal only when 'languages.java.jdk' is unresolved.
  Chained filters and 'is'/'is not' type tests work too (e.g.
  '${{ inputs.name | trim | upper }}', '${{ inputs.profile is defined }}').
#>

Set-StrictMode -Version Latest

Import-Module (Join-Path $PSScriptRoot 'Expressions.psm1') -Force

function Get-TemplateExpressionSpans {
  # Scans $Text for every '${{ ... }}' occurrence, returning each as
  # @{ Start; End (exclusive); Expression (trimmed inner text) }. Hand-rolled
  # rather than a regex so a filter argument's string literal can safely
  # contain '}}' without falsely terminating the span - tracks quote state
  # with the same backslash-escaping rule as Expressions.psm1's tokenizer.
  param([Parameter(Mandatory)][string] $Text)

  $spans = [System.Collections.Generic.List[object]]::new()
  $len = $Text.Length
  $i = 0

  while ($i -lt $len) {
    $start = $Text.IndexOf('${{', $i)
    if ($start -lt 0) { break }

    $j = $start + 3
    $quote = $null
    $end = -1
    while ($j -lt $len) {
      $c = $Text[$j]
      if ($quote) {
        if ($c -eq '\' -and ($j + 1) -lt $len) { $j += 2; continue }
        if ($c -eq $quote) { $quote = $null }
        $j++
        continue
      }
      if ($c -eq "'" -or $c -eq '"') { $quote = $c; $j++; continue }
      if ($c -eq '}' -and ($j + 1) -lt $len -and $Text[$j + 1] -eq '}') { $end = $j; break }
      $j++
    }

    if ($end -lt 0) { break } # unterminated '${{' - leave the rest of the string untouched

    $inner = $Text.Substring($start + 3, $end - ($start + 3))
    $spans.Add([pscustomobject]@{ Start = $start; End = $end + 2; Expression = $inner.Trim() })
    $i = $end + 2
  }

  return ,$spans.ToArray()
}

function ConvertTo-TemplateDisplayString {
  # Used only when an expression is interpolated into a larger string
  # (as opposed to being the string's entire value).
  param($Value)
  if ($null -eq $Value) { return '' }
  if (($Value -is [System.Collections.IEnumerable]) -and ($Value -isnot [string])) {
    return (@($Value) -join ', ')
  }
  return [string] $Value
}

function Test-ExpressionNamespaceKnown {
  # True when a Var path's top-level segment is itself a key present in
  # $Context (e.g. 'facts'/'vars'/'package'/'inputs' at the call sites that
  # provide them) - i.e. this reference belongs to a namespace this pass
  # actually knows about, as opposed to a bare id/fact name that may only
  # exist in a *later* pass's context (see the '-Soft' param below).
  param($Context, [Parameter(Mandatory)][string] $Path)
  $top = (Get-TemplatePathSegments -Path $Path)[0]
  return ($Context -is [System.Collections.IDictionary]) -and ($top -is [string]) -and $Context.Contains($top)
}

function Resolve-TemplateExpression {
  # Parses and evaluates one '${{ ... }}' expression's inner text.
  # -Soft: if ANY 'Var' reference the expression touches (there can be more
  # than one, e.g. 'a | default(b)') belongs to a namespace this pass
  # doesn't know about yet, the whole expression is left for a later pass -
  # returns @{ Handled = $false }. Otherwise returns
  # @{ Handled = $true; Value = <result, possibly $null> }.
  param([Parameter(Mandatory)][string] $Expression, [Parameter(Mandatory)] $Context, [switch] $Soft)

  $ast = Read-Expression -Expression $Expression

  if ($Soft) {
    foreach ($path in (Get-ExpressionVarPaths -Node $ast)) {
      if (-not (Test-ExpressionNamespaceKnown -Context $Context -Path $path)) { return @{ Handled = $false } }
    }
  }

  return @{ Handled = $true; Value = (Get-ExpressionValue -Node $ast -Context $Context) }
}

# A unique marker (compared by reference, never by value) returned when a
# field's *entire* value is an unresolved '${{ }}' expression - e.g.
# 'args: ${{ item.args }}' for a loop item that has no 'args'. Substituting
# an empty string there would be worse than doing nothing (wrong type,
# silently different from "field omitted" - most handlers default a missing
# optional field to '' /@()/etc. anyway), so Expand-TemplateNode instead
# removes the key/element entirely, letting the consuming code's own
# default apply. Only applies to whole-value expressions; an expression
# embedded in a larger string still blanks in place (there's no "omit part
# of a string" equivalent).
$script:TemplateOmitMarker = [pscustomobject]@{ TemplateOmitMarker = $true }

function Test-TemplateOmitMarker {
  param($Value)
  return [object]::ReferenceEquals($Value, $script:TemplateOmitMarker)
}

function Expand-TemplateValue {
  # -Soft: see Resolve-TemplateExpression. Without '-Soft' (default), every
  # unresolved reference warns+omits, matching the original behavior.
  param($Value, [Parameter(Mandatory)] $Context, [Parameter(Mandatory)][string] $PackageName, [switch] $Soft)

  if ($Value -isnot [string]) { return $Value }
  if ($Value -notmatch '\$\{\{') { return $Value }

  # Whole-value case: the entire (trimmed) field content is exactly one
  # '${{ ... }}' span - replace with the result's native type.
  $trimmed = $Value.Trim()
  $wholeSpans = Get-TemplateExpressionSpans -Text $trimmed
  if ($wholeSpans.Count -eq 1 -and $wholeSpans[0].Start -eq 0 -and $wholeSpans[0].End -eq $trimmed.Length) {
    $expr = $wholeSpans[0].Expression
    $result = Resolve-TemplateExpression -Expression $expr -Context $Context -Soft:$Soft
    if (-not $result.Handled) { return $Value }
    if ($null -eq $result.Value) {
      Write-Warning "Package '$PackageName': unresolved template reference '$expr' - field omitted"
      return $script:TemplateOmitMarker
    }
    return $result.Value
  }

  # Embedded case: one or more '${{ ... }}' spans inside a larger string -
  # substitute each as text.
  $spans = Get-TemplateExpressionSpans -Text $Value
  if ($spans.Count -eq 0) { return $Value }

  $sb = [System.Text.StringBuilder]::new()
  $cursor = 0
  foreach ($span in $spans) {
    [void] $sb.Append($Value.Substring($cursor, $span.Start - $cursor))
    $result = Resolve-TemplateExpression -Expression $span.Expression -Context $Context -Soft:$Soft
    if (-not $result.Handled) {
      [void] $sb.Append($Value.Substring($span.Start, $span.End - $span.Start))
    } elseif ($null -eq $result.Value) {
      Write-Warning "Package '$PackageName': unresolved template reference '$($span.Expression)'"
    } else {
      [void] $sb.Append((ConvertTo-TemplateDisplayString -Value $result.Value))
    }
    $cursor = $span.End
  }
  [void] $sb.Append($Value.Substring($cursor))
  return $sb.ToString()
}

function Expand-TemplateNode {
  # Recurses through hashtables/arrays produced by ConvertFrom-Yaml,
  # expanding every string leaf in place.
  param($Node, [Parameter(Mandatory)] $Context, [Parameter(Mandatory)][string] $PackageName, [switch] $Soft)

  if ($Node -is [string]) {
    return Expand-TemplateValue -Value $Node -Context $Context -PackageName $PackageName -Soft:$Soft
  }

  if ($Node -is [System.Collections.IDictionary]) {
    foreach ($key in @($Node.Keys)) {
      $resolved = Expand-TemplateNode -Node $Node[$key] -Context $Context -PackageName $PackageName -Soft:$Soft
      if (Test-TemplateOmitMarker -Value $resolved) { $Node.Remove($key) } else { $Node[$key] = $resolved }
    }
    return $Node
  }

  if ($Node -is [System.Collections.IList]) {
    for ($i = 0; $i -lt $Node.Count; $i++) {
      $resolved = Expand-TemplateNode -Node $Node[$i] -Context $Context -PackageName $PackageName -Soft:$Soft
      # No clean way to remove a list *element* mid-walk without
      # reindexing; an omitted array entry becomes $null rather than
      # vanishing (this marker is really about whole-field dict values -
      # 'args: ${{ item.args }}' - not array elements).
      $Node[$i] = if (Test-TemplateOmitMarker -Value $resolved) { $null } else { $resolved }
    }
    # ',' prevents PowerShell from unrolling this list across the function
    # return boundary - without it, a 1-element list comes back to the
    # caller as its bare single element (not a list), and a 0-element list
    # comes back as $null. Both silently break Resolve-TemplatesInPlace's
    # '$Data[$groupName] = Expand-TemplateNode ...' for exactly the
    # single-item groups this codebase has plenty of (e.g. a package with
    # one 'zip' entry).
    return ,$Node
  }

  return $Node
}

function Resolve-TemplatesInPlace {
  # Expands '${{ ... }}' expressions throughout every group/item/field of a
  # loaded package's data, in place. '-Soft': see Resolve-TemplateExpression
  # - use this for passes that only know *some* of the possible namespaces
  # (e.g. facts/vars/package/inputs, but not yet a growing id/fact
  # registry), so references to the others are left untouched for a later
  # strict pass.
  param(
    [Parameter(Mandatory)] $Data,
    [Parameter(Mandatory)] $Context,
    [Parameter(Mandatory)][string] $PackageName,
    [switch] $Soft
  )

  foreach ($groupName in @($Data.Keys)) {
    $resolved = Expand-TemplateNode -Node $Data[$groupName] -Context $Context -PackageName $PackageName -Soft:$Soft
    if (Test-TemplateOmitMarker -Value $resolved) { $Data.Remove($groupName) } else { $Data[$groupName] = $resolved }
  }
}

Export-ModuleMember -Function Resolve-TemplatesInPlace, Resolve-TemplateContext
