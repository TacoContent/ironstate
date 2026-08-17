#!/usr/bin/env pwsh
<#
.SYNOPSIS
  'when' condition evaluation for the task/action engine.

.DESCRIPTION
  A thin consumer of Expressions.psm1's shared tokenizer/parser/evaluator -
  deliberately not '${{ }}'-wrapped, so 'when' reads as bare identifiers
  (e.g. 'computer_name == "KRAYT"'), unlike the '${{ }}' string templating
  in Templates.psm1. See Expressions.psm1 for the full grammar (including
  the 'value | filter(args)' pipeline that '${{ }}' also uses - filters
  work in 'when:' too, e.g. 'when: java_version | default("25") == "25"').

  Variable paths resolve against a single flat context (facts + vars merged
  by the caller) via Expressions.psm1's Resolve-TemplateContext, the same
  dotted-path walker '${{ }}' expressions use.
#>

Set-StrictMode -Version Latest

Import-Module (Join-Path $PSScriptRoot 'Expressions.psm1') -Force

function Test-Condition {
  param([Parameter(Mandatory)][string] $Expression, [Parameter(Mandatory)][hashtable] $Context)
  $ast = Read-Expression -Expression $Expression
  return [bool] (Get-ExpressionValue -Node $ast -Context $Context)
}

function Test-WhenClause {
  # 'when' accepts a single condition string or a list of strings (list =
  # implicit AND, matching Ansible). Missing/empty/$null 'when' always passes.
  param($When, [Parameter(Mandatory)][hashtable] $Context)
  if ($null -eq $When) { return $true }
  foreach ($expr in @($When)) {
    if ([string]::IsNullOrWhiteSpace($expr)) { continue }
    if (-not (Test-Condition -Expression $expr -Context $Context)) { return $false }
  }
  return $true
}

Export-ModuleMember -Function Test-Condition, Test-WhenClause
