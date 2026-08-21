#!/usr/bin/env pwsh
<#
.SYNOPSIS
  Sandboxed, block-capable Jinja-like template engine: '{{ expr }}' output,
  '{% if/elif/else/endif %}', '{% for x in iterable %} ... {% endfor %}'
  (nesting supported), and '{% set x = expr %}'.

.DESCRIPTION
  Layers a small block-scanning/nesting parser and executor on top of
  ironstate's existing expression grammar (modules/Expressions.psm1) -
  every '{{ }}' output, 'if' condition, 'for' iterable, and 'set' value is
  parsed/evaluated by that same shared tokenizer/parser/evaluator (so the
  full filter pipeline, comparisons, 'is'/'is not', dotted paths, etc. all
  work here for free) - this module only adds the outer block structure.
  Distinct delimiters ('{{ }}'/'{% %}') from ironstate's '${{ }}' task-field
  substitution (Templates.psm1) - this engine renders file *content* read
  from a template's 'src', a completely separate pass with no collision
  risk, since it never touches YAML task fields.

  No cmdlet/command invocation exists in the underlying grammar at all, so
  there is nothing to sandbox - a template can only read/compare/filter
  values from the render context, never cause a side effect.

  Scoping: each node-list execution clones its incoming context once,
  mutates only that local clone (via 'set' or a 'for' loop variable), and
  discards it when the call returns - so a 'set' inside a 'for' body is
  isolated to that iteration, while a top-level 'set' remains visible to
  every later sibling node (including an 'if' body evaluated against the
  same, now-mutated, scope) - matching Jinja's per-block scoping.
#>

Set-StrictMode -Version Latest

Import-Module (Join-Path $PSScriptRoot '..\Expressions.psm1')
Import-Module (Join-Path $PSScriptRoot '..\Templates.psm1')

# --- tokenizer -------------------------------------------------------------

function ConvertTo-JinjaTagToken {
  # Classifies one '{% ... %}' tag's already-trimmed inner text.
  param([Parameter(Mandatory)][string] $Inner)

  if ($Inner -match '^if\s+([\s\S]+)$')   { return @{ Type = 'Tag'; Keyword = 'if';   Text = $Matches[1] } }
  if ($Inner -match '^elif\s+([\s\S]+)$') { return @{ Type = 'Tag'; Keyword = 'elif'; Text = $Matches[1] } }
  if ($Inner -match '^else\s*$')          { return @{ Type = 'Tag'; Keyword = 'else' } }
  if ($Inner -match '^endif\s*$')         { return @{ Type = 'Tag'; Keyword = 'endif' } }
  if ($Inner -match '^for\s+([A-Za-z_]\w*)\s+in\s+([\s\S]+)$') {
    return @{ Type = 'Tag'; Keyword = 'for'; VarName = $Matches[1]; Text = $Matches[2] }
  }
  if ($Inner -match '^endfor\s*$')        { return @{ Type = 'Tag'; Keyword = 'endfor' } }
  if ($Inner -match '^set\s+([A-Za-z_]\w*)\s*=\s*([\s\S]+)$') {
    return @{ Type = 'Tag'; Keyword = 'set'; VarName = $Matches[1]; Text = $Matches[2] }
  }
  throw "Unknown or malformed '{% %}' tag: '$Inner'"
}

function Get-JinjaTemplateTokens {
  # Scans $Content for '{{ ... }}' and '{% ... %}' spans, quote-aware (same
  # backslash-escaping rule as Expressions.psm1's Get-TemplateExpressionSpans)
  # so a filter argument's string literal can safely contain '}}'/'%}'
  # without falsely terminating the tag. Text between/around tags becomes
  # 'Text' tokens. An unterminated tag leaves the remainder as plain text.
  param([Parameter(Mandatory)][string] $Content)

  $tokens = [System.Collections.Generic.List[object]]::new()
  $len = $Content.Length
  $i = 0
  $textStart = 0

  while ($i -lt $len) {
    $openExpr = $Content.IndexOf('{{', $i)
    $openTag  = $Content.IndexOf('{%', $i)
    if ($openExpr -lt 0 -and $openTag -lt 0) { break }

    if ($openTag -lt 0 -or ($openExpr -ge 0 -and $openExpr -lt $openTag)) {
      $open = $openExpr; $closer = '}}'; $isTag = $false
    } else {
      $open = $openTag; $closer = '%}'; $isTag = $true
    }

    $j = $open + 2
    $quote = $null
    $end = -1
    while ($j -lt $len) {
      $c = $Content[$j]
      if ($quote) {
        if ($c -eq '\' -and ($j + 1) -lt $len) { $j += 2; continue }
        if ($c -eq $quote) { $quote = $null }
        $j++
        continue
      }
      if ($c -eq "'" -or $c -eq '"') { $quote = $c; $j++; continue }
      if ($c -eq $closer[0] -and ($j + 1) -lt $len -and $Content[$j + 1] -eq $closer[1]) { $end = $j; break }
      $j++
    }

    if ($end -lt 0) { break } # unterminated tag - leave the rest of the content untouched

    if ($open -gt $textStart) {
      $tokens.Add(@{ Type = 'Text'; Value = $Content.Substring($textStart, $open - $textStart) })
    }

    $inner = $Content.Substring($open + 2, $end - ($open + 2)).Trim()
    if ($isTag) { $tokens.Add((ConvertTo-JinjaTagToken -Inner $inner)) }
    else        { $tokens.Add(@{ Type = 'Expr'; Text = $inner }) }

    $i = $end + 2
    $textStart = $i
  }

  if ($textStart -lt $len) {
    $tokens.Add(@{ Type = 'Text'; Value = $Content.Substring($textStart) })
  }

  $tokens.Add(@{ Type = 'eof' })
  return ,$tokens.ToArray()
}

# --- block parser (recursive descent over the token list) -----------------

function New-JinjaParser {
  param([object[]] $Tokens)
  [PSCustomObject]@{ Tokens = $Tokens; Pos = 0 }
}

function Get-JinjaCurrentToken {
  param($Parser)
  return $Parser.Tokens[$Parser.Pos]
}

function Move-JinjaNext {
  param($Parser)
  $Parser.Pos++
}

function Test-JinjaTagKeyword {
  param($Token, [Parameter(Mandatory)][string] $Keyword)
  return ($Token.Type -eq 'Tag') -and ($Token.Keyword -eq $Keyword)
}

function Parse-JinjaIf {
  # Assumes the current token is the opening 'if' tag.
  param($Parser)

  $ifTok = Get-JinjaCurrentToken $Parser
  Move-JinjaNext $Parser
  $branches = [System.Collections.Generic.List[object]]::new()
  $branches.Add(@{
    ConditionAst = (Read-Expression -Expression $ifTok.Text)
    Body         = (Parse-JinjaNodeList -Parser $Parser -StopKeywords @('elif', 'else', 'endif'))
  })

  while (Test-JinjaTagKeyword (Get-JinjaCurrentToken $Parser) 'elif') {
    $elifTok = Get-JinjaCurrentToken $Parser
    Move-JinjaNext $Parser
    $branches.Add(@{
      ConditionAst = (Read-Expression -Expression $elifTok.Text)
      Body         = (Parse-JinjaNodeList -Parser $Parser -StopKeywords @('elif', 'else', 'endif'))
    })
  }

  if (Test-JinjaTagKeyword (Get-JinjaCurrentToken $Parser) 'else') {
    Move-JinjaNext $Parser
    $branches.Add(@{
      ConditionAst = $null
      Body         = (Parse-JinjaNodeList -Parser $Parser -StopKeywords @('endif'))
    })
  }

  if (-not (Test-JinjaTagKeyword (Get-JinjaCurrentToken $Parser) 'endif')) {
    throw "Missing '{% endif %}' for '{% if $($ifTok.Text) %}'"
  }
  Move-JinjaNext $Parser

  return @{ Type = 'If'; Branches = $branches.ToArray() }
}

function Parse-JinjaFor {
  # Assumes the current token is the opening 'for' tag.
  param($Parser)

  $forTok = Get-JinjaCurrentToken $Parser
  Move-JinjaNext $Parser
  $body = Parse-JinjaNodeList -Parser $Parser -StopKeywords @('endfor')

  if (-not (Test-JinjaTagKeyword (Get-JinjaCurrentToken $Parser) 'endfor')) {
    throw "Missing '{% endfor %}' for '{% for $($forTok.VarName) in $($forTok.Text) %}'"
  }
  Move-JinjaNext $Parser

  return @{
    Type        = 'For'
    VarName     = $forTok.VarName
    IterableAst = (Read-Expression -Expression $forTok.Text)
    Body        = $body
  }
}

function Parse-JinjaNodeList {
  # Consumes tokens into a flat node list until eof or a 'Tag' token whose
  # keyword is in $StopKeywords (left unconsumed, for the caller - Parse-
  # JinjaIf/-For - to see and act on). Nesting falls out for free: an inner
  # '{% for %}'/'{% if %}' met here just recurses into Parse-JinjaFor/-If.
  param($Parser, [string[]] $StopKeywords = @())

  $nodes = [System.Collections.Generic.List[object]]::new()
  while ($true) {
    $tok = Get-JinjaCurrentToken $Parser
    switch ($tok.Type) {
      'eof'  { return ,$nodes.ToArray() }
      'Text' { $nodes.Add(@{ Type = 'Text'; Value = $tok.Value }); Move-JinjaNext $Parser }
      'Expr' { $nodes.Add(@{ Type = 'Expr'; Ast = (Read-Expression -Expression $tok.Text) }); Move-JinjaNext $Parser }
      'Tag'  {
        if ($StopKeywords -contains $tok.Keyword) { return ,$nodes.ToArray() }
        switch ($tok.Keyword) {
          'if'  { $nodes.Add((Parse-JinjaIf -Parser $Parser)) }
          'for' { $nodes.Add((Parse-JinjaFor -Parser $Parser)) }
          'set' {
            $nodes.Add(@{ Type = 'Set'; VarName = $tok.VarName; ValueAst = (Read-Expression -Expression $tok.Text) })
            Move-JinjaNext $Parser
          }
          default { throw "Unexpected '{% $($tok.Keyword) %}' tag with no matching opening tag" }
        }
      }
      default { throw "Unexpected token in template" }
    }
  }
}

function ConvertTo-JinjaNodeTree {
  param([object[]] $Tokens)
  $parser = New-JinjaParser -Tokens $Tokens
  $nodes = Parse-JinjaNodeList -Parser $parser -StopKeywords @()
  $tok = Get-JinjaCurrentToken $parser
  if ($tok.Type -ne 'eof') { throw "Unexpected '{% $($tok.Keyword) %}' tag with no matching opening tag" }
  return $nodes
}

# --- executor ---------------------------------------------------------------

function Invoke-JinjaNodeList {
  # Clones $Context once into a local $scope - 'set' mutates only that
  # clone, and a 'for' iteration clones $scope again per-value - so nothing
  # here is ever visible to the caller's own context.
  param([object[]] $Nodes, [Parameter(Mandatory)][hashtable] $Context)

  $scope = @{}
  foreach ($key in @($Context.Keys)) { $scope[$key] = $Context[$key] }

  $sb = [System.Text.StringBuilder]::new()
  foreach ($node in $Nodes) {
    switch ($node.Type) {
      'Text' { [void] $sb.Append($node.Value) }
      'Expr' { [void] $sb.Append((ConvertTo-TemplateDisplayString -Value (Get-ExpressionValue -Node $node.Ast -Context $scope))) }
      'Set'  { $scope[$node.VarName] = Get-ExpressionValue -Node $node.ValueAst -Context $scope }
      'If'   {
        $chosen = $null
        foreach ($branch in $node.Branches) {
          if ($null -eq $branch.ConditionAst) { $chosen = $branch; break }
          if ([bool] (Get-ExpressionValue -Node $branch.ConditionAst -Context $scope)) { $chosen = $branch; break }
        }
        if ($chosen) { [void] $sb.Append((Invoke-JinjaNodeList -Nodes $chosen.Body -Context $scope)) }
      }
      'For'  {
        $iterable = Get-ExpressionValue -Node $node.IterableAst -Context $scope
        foreach ($value in @($iterable)) {
          $child = @{}
          foreach ($key in @($scope.Keys)) { $child[$key] = $scope[$key] }
          $child[$node.VarName] = $value
          [void] $sb.Append((Invoke-JinjaNodeList -Nodes $node.Body -Context $child))
        }
      }
      default { throw "Cannot render node of type '$($node.Type)'" }
    }
  }
  return $sb.ToString()
}

function Render-JinjaTemplate {
  param([Parameter(Mandatory)][string] $Content, [Parameter(Mandatory)][hashtable] $Context)
  $tokens = Get-JinjaTemplateTokens -Content $Content
  $nodes = ConvertTo-JinjaNodeTree -Tokens $tokens
  return Invoke-JinjaNodeList -Nodes $nodes -Context $Context
}

Export-ModuleMember -Function Render-JinjaTemplate
