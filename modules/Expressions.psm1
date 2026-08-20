#!/usr/bin/env pwsh
<#
.SYNOPSIS
  Shared expression tokenizer/parser/evaluator for 'when' conditions
  (Conditions.psm1) and '${{ ... }}' template substitution (Templates.psm1).

.DESCRIPTION
  A small Ansible/Jinja-flavored expression language. One grammar/AST backs
  both consumers so they can't drift apart:
    - Conditions.psm1 parses a bare expression (e.g. 'computer_name == "KRAYT"')
      and casts the result to [bool] for 'when:'.
    - Templates.psm1 parses the text inside '${{ ... }}' and keeps the
      result's native type (string, array, bool, ...) for template
      substitution.

  Grammar (lowest to highest precedence):
    expr        := or_expr
    or_expr     := and_expr ("or" and_expr)*
    and_expr    := not_expr ("and" not_expr)*
    not_expr    := "not" not_expr | comparison
    comparison  := membership (("==" | "!=" | "<" | "<=" | ">" | ">=") membership)?
    membership  := pipeline (("in" | "not" "in") primary | ("is" | "is not") IDENT)?
    pipeline    := primary ("|" IDENT ("(" (expr ("," expr)*)? ")")?)*
    primary     := STRING | NUMBER | "true" | "false" | "null"
                 | "[" (expr ("," expr)*)? "]"
                 | IDENT (("." IDENT) | ("[" NUMBER "]"))*   (dotted/indexed variable path)
                 | "(" expr ")"

  'pipeline' is a Jinja-style filter chain: 'value | default(x)',
  'value | trim | upper', etc. - see $script:ExpressionFilters below for the
  filter registry, populated from modules/Filters/ (one file per filter -
  add a new filter by adding a file there, nothing else to wire up). It
  binds tighter than comparisons, so 'a | default("x") == "x"' filters
  first, then compares.

  'is'/'is not' tests (Jinja-flavored) check a resolved value's runtime type
  rather than its truthiness - needed because '==' and bare truthy checks
  both go through an explicit [bool] cast (see Test-ExpressionValuesEqual),
  and .NET casts *any* non-null hashtable to $true regardless of its
  contents. That makes '==' unable to tell "boolean true" apart from "a
  map" - e.g. 'languages == true' would also be true for
  'languages: { rust: false }'. 'languages is mapping' / 'languages is
  boolean' test the actual type instead, so a config value can be either a
  blanket boolean or a per-key map (see packages/languages/main.yml).
  Supported test names: mapping (alias: map), boolean (alias: bool),
  string, number, list, defined, none (alias: null).

  String equality/ordering is case-sensitive (ordinal), matching
  Ansible/Jinja - PowerShell's native -eq/-lt are case-insensitive, so
  comparisons here are hand-rolled rather than delegated to them.
#>

Set-StrictMode -Version Latest

# --- variable path resolution (shared primitive) ---------------------------

# Path segments: a bare identifier ('foo'), a dotted one ('.foo'), or a
# bracketed integer index ('[0]') - e.g. 'example_task.results[0].rc'.
$script:PathSegmentPattern = '([A-Za-z0-9_]+)|\[([0-9]+)\]'

function Get-TemplatePathSegments {
  # Tokenizes a path string like 'a.b[0].c' into an ordered list of
  # segments: strings for dict-key access, ints for list-index access.
  param([Parameter(Mandatory)][string] $Path)
  $segments = [System.Collections.Generic.List[object]]::new()
  foreach ($m in [regex]::Matches($Path, $script:PathSegmentPattern)) {
    if ($m.Groups[2].Success) { $segments.Add([int] $m.Groups[2].Value) } else { $segments.Add($m.Groups[1].Value) }
  }
  return ,$segments.ToArray()
}

function Resolve-TemplateContext {
  # Walks a dotted/indexed path (e.g. 'inputs.profile' or
  # 'example_task.results[0].rc') through nested dictionaries and lists.
  # Returns $null if any segment is missing or out of range.
  param($Context, [Parameter(Mandatory)][string] $Path)

  $current = $Context
  foreach ($part in (Get-TemplatePathSegments -Path $Path)) {
    if ($null -eq $current) { return $null }
    if ($part -is [int]) {
      if ($current -isnot [System.Collections.IList] -or $part -lt 0 -or $part -ge $current.Count) { return $null }
      $current = $current[$part]
    } else {
      if ($current -isnot [System.Collections.IDictionary] -or -not $current.Contains($part)) { return $null }
      $current = $current[$part]
    }
  }
  return $current
}

# --- filter registry ---------------------------------------------------

# Populated below from modules/Filters/*.ps1 - one file per filter, named
# for the filter itself (e.g. 'Filters/upper.ps1' -> the 'upper' filter).
# Each file is a plain script (its own 'param($Value, [object[]] $ArgValues)'
# block), invoked positionally by Invoke-ExpressionFilter - so adding a new
# filter is just adding a file here, no registry entry to hand-wire.
$script:ExpressionFilters = @{}
Get-ChildItem -Path (Join-Path $PSScriptRoot 'Filters') -Filter '*.ps1' | ForEach-Object {
  $script:ExpressionFilters[$_.BaseName] = $_.FullName
}

function Invoke-ExpressionFilter {
  param([Parameter(Mandatory)][string] $Name, $Value, [object[]] $ArgValues)
  if (-not $script:ExpressionFilters.Contains($Name)) { throw "Unknown filter '$Name'" }
  return & $script:ExpressionFilters[$Name] $Value $ArgValues
}

# --- tokenizer ---------------------------------------------------------

function ConvertTo-ExpressionTokens {
  param([Parameter(Mandatory)][string] $Expression)

  $tokens = [System.Collections.Generic.List[object]]::new()
  $i = 0
  $len = $Expression.Length

  while ($i -lt $len) {
    $c = $Expression[$i]

    if ($c -match '\s') { $i++; continue }

    if ($c -eq '(') { $tokens.Add(@{ Type = 'lparen' }); $i++; continue }
    if ($c -eq ')') { $tokens.Add(@{ Type = 'rparen' }); $i++; continue }
    if ($c -eq '[') { $tokens.Add(@{ Type = 'lbracket' }); $i++; continue }
    if ($c -eq ']') { $tokens.Add(@{ Type = 'rbracket' }); $i++; continue }
    if ($c -eq ',') { $tokens.Add(@{ Type = 'comma' }); $i++; continue }
    if ($c -eq '.') { $tokens.Add(@{ Type = 'dot' }); $i++; continue }
    if ($c -eq '|') { $tokens.Add(@{ Type = 'pipe' }); $i++; continue }

    if ($c -eq '=' -and ($i + 1) -lt $len -and $Expression[$i + 1] -eq '=') { $tokens.Add(@{ Type = 'eq' }); $i += 2; continue }
    if ($c -eq '!' -and ($i + 1) -lt $len -and $Expression[$i + 1] -eq '=') { $tokens.Add(@{ Type = 'ne' }); $i += 2; continue }
    if ($c -eq '<' -and ($i + 1) -lt $len -and $Expression[$i + 1] -eq '=') { $tokens.Add(@{ Type = 'le' }); $i += 2; continue }
    if ($c -eq '>' -and ($i + 1) -lt $len -and $Expression[$i + 1] -eq '=') { $tokens.Add(@{ Type = 'ge' }); $i += 2; continue }
    if ($c -eq '<') { $tokens.Add(@{ Type = 'lt' }); $i++; continue }
    if ($c -eq '>') { $tokens.Add(@{ Type = 'gt' }); $i++; continue }

    if ($c -eq "'" -or $c -eq '"') {
      $quote = $c
      $j = $i + 1
      $sb = [System.Text.StringBuilder]::new()
      while ($j -lt $len -and $Expression[$j] -ne $quote) {
        if ($Expression[$j] -eq '\' -and ($j + 1) -lt $len) { [void] $sb.Append($Expression[$j + 1]); $j += 2 }
        else { [void] $sb.Append($Expression[$j]); $j++ }
      }
      if ($j -ge $len) { throw "Unterminated string literal in expression: $Expression" }
      $tokens.Add(@{ Type = 'string'; Value = $sb.ToString() })
      $i = $j + 1
      continue
    }

    if ($c -match '[0-9]' -or ($c -eq '-' -and ($i + 1) -lt $len -and $Expression[$i + 1] -match '[0-9]')) {
      $j = $i + 1
      while ($j -lt $len -and $Expression[$j] -match '[0-9.]') { $j++ }
      $numText = $Expression.Substring($i, $j - $i)
      $tokens.Add(@{ Type = 'number'; Value = [double] $numText })
      $i = $j
      continue
    }

    if ($c -match '[A-Za-z_]') {
      $j = $i
      while ($j -lt $len -and $Expression[$j] -match '[A-Za-z0-9_]') { $j++ }
      $word = $Expression.Substring($i, $j - $i)
      switch ($word) {
        'and'   { $tokens.Add(@{ Type = 'and' }) }
        'or'    { $tokens.Add(@{ Type = 'or' }) }
        'not'   { $tokens.Add(@{ Type = 'not' }) }
        'in'    { $tokens.Add(@{ Type = 'in' }) }
        'is'    { $tokens.Add(@{ Type = 'is' }) }
        'true'  { $tokens.Add(@{ Type = 'bool'; Value = $true }) }
        'false' { $tokens.Add(@{ Type = 'bool'; Value = $false }) }
        'null'  { $tokens.Add(@{ Type = 'null' }) }
        default { $tokens.Add(@{ Type = 'ident'; Value = $word }) }
      }
      $i = $j
      continue
    }

    throw "Unexpected character '$c' in expression: $Expression"
  }

  $tokens.Add(@{ Type = 'eof' })
  return ,$tokens.ToArray()
}

# --- parser (recursive descent; builds a small AST of hashtables) ---------

function New-ExpressionParser {
  param([object[]] $Tokens)
  [PSCustomObject]@{ Tokens = $Tokens; Pos = 0 }
}

function Get-ExpressionCurrentToken {
  param($Parser)
  return $Parser.Tokens[$Parser.Pos]
}

function Move-ExpressionNext {
  param($Parser)
  $Parser.Pos++
}

$script:ExpressionCompareOps = @{ eq = '=='; ne = '!='; lt = '<'; le = '<='; gt = '>'; ge = '>=' }

function Parse-ExpressionExpr {
  param($Parser)
  Parse-ExpressionOr -Parser $Parser
}

function Parse-ExpressionOr {
  param($Parser)
  $left = Parse-ExpressionAnd -Parser $Parser
  while ((Get-ExpressionCurrentToken $Parser).Type -eq 'or') {
    Move-ExpressionNext $Parser
    $right = Parse-ExpressionAnd -Parser $Parser
    $left = @{ Type = 'Or'; Left = $left; Right = $right }
  }
  return $left
}

function Parse-ExpressionAnd {
  param($Parser)
  $left = Parse-ExpressionNot -Parser $Parser
  while ((Get-ExpressionCurrentToken $Parser).Type -eq 'and') {
    Move-ExpressionNext $Parser
    $right = Parse-ExpressionNot -Parser $Parser
    $left = @{ Type = 'And'; Left = $left; Right = $right }
  }
  return $left
}

function Parse-ExpressionNot {
  param($Parser)
  if ((Get-ExpressionCurrentToken $Parser).Type -eq 'not') {
    Move-ExpressionNext $Parser
    $operand = Parse-ExpressionNot -Parser $Parser
    return @{ Type = 'Not'; Operand = $operand }
  }
  return Parse-ExpressionComparison -Parser $Parser
}

function Parse-ExpressionComparison {
  param($Parser)
  $left = Parse-ExpressionMembership -Parser $Parser
  $tokType = (Get-ExpressionCurrentToken $Parser).Type
  if ($script:ExpressionCompareOps.Contains($tokType)) {
    Move-ExpressionNext $Parser
    $right = Parse-ExpressionMembership -Parser $Parser
    return @{ Type = 'Compare'; Op = $script:ExpressionCompareOps[$tokType]; Left = $left; Right = $right }
  }
  return $left
}

function Parse-ExpressionMembership {
  param($Parser)
  $left = Parse-ExpressionPipeline -Parser $Parser
  $tok = Get-ExpressionCurrentToken $Parser

  if ($tok.Type -eq 'in') {
    Move-ExpressionNext $Parser
    $right = Parse-ExpressionPrimary -Parser $Parser
    return @{ Type = 'In'; Negate = $false; Left = $left; Right = $right }
  }

  if ($tok.Type -eq 'not') {
    $savedPos = $Parser.Pos
    Move-ExpressionNext $Parser
    if ((Get-ExpressionCurrentToken $Parser).Type -eq 'in') {
      Move-ExpressionNext $Parser
      $right = Parse-ExpressionPrimary -Parser $Parser
      return @{ Type = 'In'; Negate = $true; Left = $left; Right = $right }
    }
    $Parser.Pos = $savedPos
  }

  if ($tok.Type -eq 'is') {
    Move-ExpressionNext $Parser
    $negate = $false
    if ((Get-ExpressionCurrentToken $Parser).Type -eq 'not') {
      Move-ExpressionNext $Parser
      $negate = $true
    }
    $testTok = Get-ExpressionCurrentToken $Parser
    if ($testTok.Type -ne 'ident') { throw "Expected a test name after 'is' in expression" }
    Move-ExpressionNext $Parser
    return @{ Type = 'Is'; Negate = $negate; Left = $left; Test = $testTok.Value }
  }

  return $left
}

function Parse-ExpressionPipeline {
  # 'value | filter(args)' filter chain - see $script:ExpressionFilters.
  # Binds tighter than comparisons/membership so 'a | default("x") == "x"'
  # filters 'a' first, then compares.
  param($Parser)
  $left = Parse-ExpressionPrimary -Parser $Parser
  while ((Get-ExpressionCurrentToken $Parser).Type -eq 'pipe') {
    Move-ExpressionNext $Parser
    $nameTok = Get-ExpressionCurrentToken $Parser
    if ($nameTok.Type -ne 'ident') { throw "Expected a filter name after '|' in expression" }
    Move-ExpressionNext $Parser

    $filterArgs = [System.Collections.Generic.List[object]]::new()
    if ((Get-ExpressionCurrentToken $Parser).Type -eq 'lparen') {
      Move-ExpressionNext $Parser
      if ((Get-ExpressionCurrentToken $Parser).Type -ne 'rparen') {
        $filterArgs.Add((Parse-ExpressionExpr -Parser $Parser))
        while ((Get-ExpressionCurrentToken $Parser).Type -eq 'comma') {
          Move-ExpressionNext $Parser
          $filterArgs.Add((Parse-ExpressionExpr -Parser $Parser))
        }
      }
      if ((Get-ExpressionCurrentToken $Parser).Type -ne 'rparen') { throw "Expected ')' after filter arguments in expression" }
      Move-ExpressionNext $Parser
    }

    $left = @{ Type = 'Filter'; Name = $nameTok.Value; Args = $filterArgs.ToArray(); Left = $left }
  }
  return $left
}

function Parse-ExpressionPrimary {
  param($Parser)
  $tok = Get-ExpressionCurrentToken $Parser

  switch ($tok.Type) {
    'string' { Move-ExpressionNext $Parser; return @{ Type = 'Literal'; Value = $tok.Value } }
    'number' { Move-ExpressionNext $Parser; return @{ Type = 'Literal'; Value = $tok.Value } }
    'bool'   { Move-ExpressionNext $Parser; return @{ Type = 'Literal'; Value = $tok.Value } }
    'null'   { Move-ExpressionNext $Parser; return @{ Type = 'Literal'; Value = $null } }
    'lparen' {
      Move-ExpressionNext $Parser
      $inner = Parse-ExpressionExpr -Parser $Parser
      if ((Get-ExpressionCurrentToken $Parser).Type -ne 'rparen') { throw "Expected ')' in expression" }
      Move-ExpressionNext $Parser
      return $inner
    }
    'lbracket' {
      Move-ExpressionNext $Parser
      $items = [System.Collections.Generic.List[object]]::new()
      if ((Get-ExpressionCurrentToken $Parser).Type -ne 'rbracket') {
        $items.Add((Parse-ExpressionExpr -Parser $Parser))
        while ((Get-ExpressionCurrentToken $Parser).Type -eq 'comma') {
          Move-ExpressionNext $Parser
          $items.Add((Parse-ExpressionExpr -Parser $Parser))
        }
      }
      if ((Get-ExpressionCurrentToken $Parser).Type -ne 'rbracket') { throw "Expected ']' in expression" }
      Move-ExpressionNext $Parser
      return @{ Type = 'List'; Items = $items.ToArray() }
    }
    'ident' {
      # Built as a single dotted/bracketed path string (e.g.
      # 'example_task.results[0].rc'), not an array of segments - that's
      # what Resolve-TemplateContext/Get-TemplatePathSegments above already
      # know how to walk.
      $path = [System.Text.StringBuilder]::new()
      [void] $path.Append($tok.Value)
      Move-ExpressionNext $Parser
      while ($true) {
        $cur = Get-ExpressionCurrentToken $Parser
        if ($cur.Type -eq 'dot') {
          Move-ExpressionNext $Parser
          $next = Get-ExpressionCurrentToken $Parser
          if ($next.Type -ne 'ident') { throw "Expected identifier after '.' in expression" }
          [void] $path.Append('.').Append($next.Value)
          Move-ExpressionNext $Parser
        } elseif ($cur.Type -eq 'lbracket') {
          Move-ExpressionNext $Parser
          $idx = Get-ExpressionCurrentToken $Parser
          if ($idx.Type -ne 'number') { throw "Expected a number inside '[...]' in expression" }
          Move-ExpressionNext $Parser
          if ((Get-ExpressionCurrentToken $Parser).Type -ne 'rbracket') { throw "Expected ']' in expression" }
          Move-ExpressionNext $Parser
          [void] $path.Append('[').Append([int] $idx.Value).Append(']')
        } else {
          break
        }
      }
      return @{ Type = 'Var'; Path = $path.ToString() }
    }
    default { throw "Unexpected token '$($tok.Type)' in expression" }
  }
}

function Read-Expression {
  # Tokenize + parse + require the whole string was consumed. The single
  # public entry point for turning expression text into an AST.
  param([Parameter(Mandatory)][string] $Expression)
  $tokens = ConvertTo-ExpressionTokens -Expression $Expression
  $parser = New-ExpressionParser -Tokens $tokens
  $ast = Parse-ExpressionExpr -Parser $parser
  if ((Get-ExpressionCurrentToken $parser).Type -ne 'eof') { throw "Unexpected trailing input in expression: $Expression" }
  return $ast
}

# --- evaluator -----------------------------------------------------------

function Test-ExpressionValuesEqual {
  # Type-aware, case-sensitive equality (bool/number/ordinal-string), unlike
  # PowerShell's native -eq which is case-insensitive for strings.
  param($A, $B)
  if ($null -eq $A -or $null -eq $B) { return ($null -eq $A) -and ($null -eq $B) }
  if ($A -is [bool] -or $B -is [bool]) { return [bool] $A -eq [bool] $B }
  if (($A -is [double] -or $A -is [int]) -and ($B -is [double] -or $B -is [int])) { return [double] $A -eq [double] $B }
  return [string]::CompareOrdinal([string] $A, [string] $B) -eq 0
}

function Compare-ExpressionValues {
  # Numeric compare when both sides look numeric; ordinal string compare otherwise.
  param($A, $B)
  if (($A -is [double] -or $A -is [int]) -and ($B -is [double] -or $B -is [int])) {
    $da = [double] $A; $db = [double] $B
    if ($da -lt $db) { return -1 }
    if ($da -gt $db) { return 1 }
    return 0
  }
  return [string]::CompareOrdinal([string] $A, [string] $B)
}

function Test-ExpressionTypeTest {
  # Backs 'is'/'is not' - a runtime type check, unlike '==' which coerces
  # through [bool] and can't tell a boolean apart from a hashtable.
  param($Value, [Parameter(Mandatory)][string] $Test)
  switch ($Test) {
    { $_ -in 'mapping', 'map' }     { return $Value -is [System.Collections.IDictionary] }
    { $_ -in 'boolean', 'bool' }    { return $Value -is [bool] }
    'string'                        { return $Value -is [string] }
    'number'                        { return ($Value -is [double]) -or ($Value -is [int]) }
    'list'                          { return ($Value -is [System.Collections.IEnumerable]) -and ($Value -isnot [string]) -and ($Value -isnot [System.Collections.IDictionary]) }
    'defined'                       { return $null -ne $Value }
    { $_ -in 'none', 'null' }       { return $null -eq $Value }
    default                         { throw "Unknown expression test 'is $Test'" }
  }
}

function Get-ExpressionValue {
  # Resolves any AST node to its runtime value. Boolean-shaped nodes
  # (And/Or/Not/Compare/In/Is) just return a real [bool] - this one
  # function backs both 'when:' truthiness (Conditions.psm1 casts the
  # final result to [bool]) and '${{ }}' value substitution (Templates.psm1
  # keeps the native type).
  param($Node, [Parameter(Mandatory)] $Context)

  switch ($Node.Type) {
    'Literal' { return $Node.Value }
    'Var'     { return Resolve-TemplateContext -Context $Context -Path $Node.Path }
    'List'    {
      # Built via .Add(), not '@(... | ForEach-Object ...)' - a 0- or
      # 1-item list literal (e.g. '[]', or '[x]' where x itself evaluates
      # to an array) would otherwise be unrolled into zero/one *bare*
      # output objects crossing this function's own return boundary,
      # rather than surviving as one list-shaped value - same hazard as
      # the 'Filter' case just below.
      $items = [System.Collections.Generic.List[object]]::new()
      foreach ($itemNode in $Node.Items) { $items.Add((Get-ExpressionValue -Node $itemNode -Context $Context)) }
      return ,$items.ToArray()
    }
    'Filter'  {
      # Built via .Add(), not '@(... | ForEach-Object ...)' - an argument
      # that itself evaluates to an array (e.g. 'default([])') would
      # otherwise be unrolled by the pipe operator, silently vanishing from
      # $argValues instead of surviving as one (empty-array-valued)
      # argument - same enumeration hazard called out in Tasks.psm1's
      # 'with'/'items' handling.
      $value = Get-ExpressionValue -Node $Node.Left -Context $Context
      $argValues = [System.Collections.Generic.List[object]]::new()
      foreach ($argNode in $Node.Args) { $argValues.Add((Get-ExpressionValue -Node $argNode -Context $Context)) }
      return Invoke-ExpressionFilter -Name $Node.Name -Value $value -ArgValues $argValues.ToArray()
    }
    'Or'  { return (Get-ExpressionValue -Node $Node.Left -Context $Context) -or (Get-ExpressionValue -Node $Node.Right -Context $Context) }
    'And' { return (Get-ExpressionValue -Node $Node.Left -Context $Context) -and (Get-ExpressionValue -Node $Node.Right -Context $Context) }
    'Not' { return -not (Get-ExpressionValue -Node $Node.Operand -Context $Context) }
    'Compare' {
      $a = Get-ExpressionValue -Node $Node.Left -Context $Context
      $b = Get-ExpressionValue -Node $Node.Right -Context $Context
      switch ($Node.Op) {
        '==' { return Test-ExpressionValuesEqual -A $a -B $b }
        '!=' { return -not (Test-ExpressionValuesEqual -A $a -B $b) }
        '<'  { return (Compare-ExpressionValues -A $a -B $b) -lt 0 }
        '<=' { return (Compare-ExpressionValues -A $a -B $b) -le 0 }
        '>'  { return (Compare-ExpressionValues -A $a -B $b) -gt 0 }
        '>=' { return (Compare-ExpressionValues -A $a -B $b) -ge 0 }
      }
    }
    'In' {
      $needle = Get-ExpressionValue -Node $Node.Left -Context $Context
      $haystack = Get-ExpressionValue -Node $Node.Right -Context $Context
      $result =
        if ($haystack -is [string]) { $haystack.Contains([string] $needle) }
        elseif ($haystack -is [System.Collections.IEnumerable]) {
          $found = $false
          foreach ($item in $haystack) { if (Test-ExpressionValuesEqual -A $needle -B $item) { $found = $true; break } }
          $found
        } else { $false }
      if ($Node.Negate) { return -not $result } else { return $result }
    }
    'Is' {
      $value = Get-ExpressionValue -Node $Node.Left -Context $Context
      $result = Test-ExpressionTypeTest -Value $value -Test $Node.Test
      if ($Node.Negate) { return -not $result } else { return $result }
    }
    default { throw "Cannot evaluate node of type '$($Node.Type)'" }
  }
}

function Add-ExpressionVarPaths {
  # Internal recursion helper for Get-ExpressionVarPaths - appends into
  # $Paths rather than returning arrays, to sidestep PowerShell pipeline
  # unrolling ambiguity with nested array results.
  param($Node, [System.Collections.Generic.List[string]] $Paths)
  if ($null -eq $Node) { return }
  switch ($Node.Type) {
    'Var'     { $Paths.Add($Node.Path) }
    'List'    { foreach ($item in $Node.Items) { Add-ExpressionVarPaths -Node $item -Paths $Paths } }
    'Filter'  {
      Add-ExpressionVarPaths -Node $Node.Left -Paths $Paths
      foreach ($a in $Node.Args) { Add-ExpressionVarPaths -Node $a -Paths $Paths }
    }
    'Or'      { Add-ExpressionVarPaths -Node $Node.Left -Paths $Paths; Add-ExpressionVarPaths -Node $Node.Right -Paths $Paths }
    'And'     { Add-ExpressionVarPaths -Node $Node.Left -Paths $Paths; Add-ExpressionVarPaths -Node $Node.Right -Paths $Paths }
    'Not'     { Add-ExpressionVarPaths -Node $Node.Operand -Paths $Paths }
    'Compare' { Add-ExpressionVarPaths -Node $Node.Left -Paths $Paths; Add-ExpressionVarPaths -Node $Node.Right -Paths $Paths }
    'In'      { Add-ExpressionVarPaths -Node $Node.Left -Paths $Paths; Add-ExpressionVarPaths -Node $Node.Right -Paths $Paths }
    'Is'      { Add-ExpressionVarPaths -Node $Node.Left -Paths $Paths }
    default   { } # 'Literal' has no variable references
  }
}

function Get-ExpressionVarPaths {
  # Walks an AST and returns every 'Var' node's dotted path string - used by
  # Templates.psm1's soft pass to decide whether a *compound* '${{ }}'
  # expression (e.g. 'a | default(b)', with two Var references) is fully
  # resolvable yet, generalizing what used to be a single top-level check.
  param($Node)
  $paths = [System.Collections.Generic.List[string]]::new()
  Add-ExpressionVarPaths -Node $Node -Paths $paths
  return ,$paths.ToArray()
}

Export-ModuleMember -Function Resolve-TemplateContext, Get-TemplatePathSegments, ConvertTo-ExpressionTokens, Read-Expression, Get-ExpressionValue, Get-ExpressionVarPaths
