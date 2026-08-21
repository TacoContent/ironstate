#!/usr/bin/env pwsh
<#
.SYNOPSIS
  Handler for the 'assert' module: fails the task unless every 'that'
  condition holds.

.DESCRIPTION
  { that: <condition string, or list>, fail_msg: <string>, success_msg: <string> }.
  'that' uses the same bare-expression grammar as 'when' (Conditions.psm1/
  Expressions.psm1) - dotted/indexed identifiers, ==, !=, <, <=, >, >=, and,
  or, not, in/not in, is/is not, and '|' filters - evaluated against this
  leaf's flat context (facts/vars/id-registered results), same as 'when'. A
  single string or a list (implicit AND, matching 'when') are both accepted;
  a missing/empty 'that' vacuously passes, same as an empty 'when'.

  Reuses Log.psm1/Fact.psm1's trick for a module with no real idempotent
  "already applied" state: Test always reports "not installed", so this
  always resolves to Install (the check always runs when reached) - 'state'
  is not a meaningful field here and is ignored.

  Unlike Log/Fact, an assert's whole purpose is the check itself, so
  ironstate.ps1 forces it to actually run even under dry-run (no '-Apply') -
  the same exception already made for a fact's embedded 'shell' - rather
  than only printing a description and skipping the real evaluation.

  Pass/fail becomes this leaf's rc (0/1), which is exactly what the existing
  generic 'failed_when'/'continue_on_error' machinery in ironstate.ps1's
  Invoke-Tasks already acts on - no special-cased stop-the-run logic is
  needed here, only a correctly-shaped exec result.

  'fail_msg'/'success_msg' are used verbatim when given; otherwise a message
  is built from the task's name and (for a failure) every 'that' condition
  that didn't hold - not just the first, so a multi-condition assert reports
  everything wrong at once rather than making the author fix-and-rerun one
  condition at a time.
#>

Set-StrictMode -Version Latest

Import-Module (Join-Path $PSScriptRoot '..\Common.psm1')
Import-Module (Join-Path $PSScriptRoot '..\Conditions.psm1')

function Get-AssertFailedConditions {
  param($That, [Parameter(Mandatory)][hashtable] $Context)
  $failed = [System.Collections.Generic.List[string]]::new()
  foreach ($expr in @($That)) {
    if ([string]::IsNullOrWhiteSpace($expr)) { continue }
    if (-not (Test-Condition -Expression $expr -Context $Context)) { $failed.Add($expr) }
  }
  return ,$failed.ToArray()
}

function Get-AssertHandler {
  [PSCustomObject]@{
    Test      = {
      param($Item)
      $false
    }
    Describe  = {
      param($Item, $Action, $Context)
      $that = @(Get-Prop $Item 'that' @())
      "assert: " + ($that -join ' && ')
    }
    Install   = {
      param($Item, $Name, $Context)
      $that = @(Get-Prop $Item 'that' @())
      $label = if ($Name) { $Name } else { '<unnamed>' }
      $failedConditions = Get-AssertFailedConditions -That $that -Context $Context

      if ($failedConditions.Count -gt 0) {
        $message = Get-Prop $Item 'fail_msg'
        if (-not $message) {
          $message = "Assertion failed for task '$label': $($failedConditions -join '; ')"
        }
        Write-Warning $message
        return @{ rc = 1; stdout = ''; stdout_lines = @(); stderr = $message; stderr_lines = @($message) }
      }

      $message = Get-Prop $Item 'success_msg'
      if (-not $message) {
        $message = "Assertion passed for task '$label' ($($that.Count) condition(s))."
      }
      Write-Host $message
      return @{ rc = 0; stdout = $message; stdout_lines = @($message); stderr = ''; stderr_lines = @() }
    }
    Uninstall = {
      param($Item, $Name, $Context)
      # Never reached: Test always reports "not installed", so
      # Resolve-PackageAction never resolves this module to Uninstall.
    }
  }
}

Export-ModuleMember -Function Get-AssertHandler
