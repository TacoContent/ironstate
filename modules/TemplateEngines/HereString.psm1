#!/usr/bin/env pwsh
<#
.SYNOPSIS
  Restricted, native-here-string-flavored template engine: bare
  '$Name.Path' and '$(...)' interpolation only - no block constructs.

.DESCRIPTION
  Looks like a PowerShell expandable here-string (@"..."@), but is not one
  - it never calls Invoke-Expression or otherwise runs real PowerShell.
  Both '$Name.Path' (no parens) and '$(...)' spans are evaluated through
  the exact same restricted grammar/evaluator as the 'jinja' engine
  (modules/Expressions.psm1's Read-Expression/Get-ExpressionValue) - a
  bare dotted path is already a complete, valid expression under that
  grammar on its own, so there's no separate "path mode" vs. "expression
  mode". Since that grammar has no cmdlet-invocation production at all,
  there is nothing to sandbox: a template can only read/compare/filter
  context values, never cause a side effect. No 'if'/'for' blocks exist
  here, matching real here-string semantics (interpolation only) - this is
  the "simple substitution" tier, with 'jinja' as the "full sandboxed
  logic" tier and 'eps' as the "full PowerShell power" tier.
#>

Set-StrictMode -Version Latest

Import-Module (Join-Path $PSScriptRoot '..\Expressions.psm1')
Import-Module (Join-Path $PSScriptRoot '..\Templates.psm1')

function Get-HereStringExpressionSpans {
  # Scans $Text for '$(...)' and bare '$Name.Path'/'$Name[0]' spans,
  # returning each as @{ Start; End (exclusive); InnerText }. A '$(' span
  # tracks paren-depth *and* quote state (same backslash-escaping rule used
  # elsewhere in this codebase) so a filter call's own parens/quotes inside
  # don't falsely close it. An unterminated '$(' leaves the remainder
  # untouched, matching Templates.psm1's Get-TemplateExpressionSpans
  # convention. A lone '$' not followed by '(' or an identifier character
  # is left as a literal '$', not a span.
  param([Parameter(Mandatory)][string] $Text)

  $spans = [System.Collections.Generic.List[object]]::new()
  $len = $Text.Length
  $i = 0

  while ($i -lt $len) {
    $dollar = $Text.IndexOf('$', $i)
    if ($dollar -lt 0) { break }

    if (($dollar + 1) -lt $len -and $Text[$dollar + 1] -eq '(') {
      $j = $dollar + 2
      $depth = 1
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
        if ($c -eq '(') { $depth++; $j++; continue }
        if ($c -eq ')') {
          $depth--
          if ($depth -eq 0) { $end = $j; break }
          $j++
          continue
        }
        $j++
      }

      if ($end -lt 0) { break } # unterminated '$(' - leave the rest of the text untouched

      $spans.Add([pscustomobject]@{
        Start     = $dollar
        End       = $end + 1
        InnerText = $Text.Substring($dollar + 2, $end - ($dollar + 2))
      })
      $i = $end + 1
      continue
    }

    if (($dollar + 1) -lt $len -and $Text[$dollar + 1] -match '[A-Za-z_]') {
      $m = [regex]::Match($Text.Substring($dollar + 1), '^[A-Za-z_]\w*(?:\.[A-Za-z_]\w*|\[[0-9]+\])*')
      $spans.Add([pscustomobject]@{
        Start     = $dollar
        End       = $dollar + 1 + $m.Value.Length
        InnerText = $m.Value
      })
      $i = $dollar + 1 + $m.Value.Length
      continue
    }

    # Lone '$' (end of string, or not followed by '(' / an identifier) - literal.
    $i = $dollar + 1
  }

  return ,$spans.ToArray()
}

function Render-HereStringTemplate {
  param([Parameter(Mandatory)][string] $Content, [Parameter(Mandatory)][hashtable] $Context)

  $spans = Get-HereStringExpressionSpans -Text $Content
  if ($spans.Count -eq 0) { return $Content }

  $sb = [System.Text.StringBuilder]::new()
  $cursor = 0
  foreach ($span in $spans) {
    [void] $sb.Append($Content.Substring($cursor, $span.Start - $cursor))
    $value = Get-ExpressionValue -Node (Read-Expression -Expression $span.InnerText) -Context $Context
    [void] $sb.Append((ConvertTo-TemplateDisplayString -Value $value))
    $cursor = $span.End
  }
  [void] $sb.Append($Content.Substring($cursor))
  return $sb.ToString()
}

Export-ModuleMember -Function Render-HereStringTemplate
