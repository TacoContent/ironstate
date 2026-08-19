#!/usr/bin/env pwsh
<#
.SYNOPSIS
  Handler for the 'fact' module: sets an arbitrary named value for later
  tasks to reference.

.DESCRIPTION
  { name: <string>, value: <any YAML value>, shell: <shellItem>, state: present|absent }.
  'value' can be a scalar, a list, or a nested mapping - stored as-is (it's
  already been through the per-leaf '${{ }}' template pass by the time this
  runs, so e.g. 'value: "${{ some_id }}"' resolves to that id's registered
  result).

  'shell' lets a fact compute its own value by running a command, instead of
  relying on a separately-'id'd shell task's registered result - the command
  always actually runs, even without '-Apply' (see below for why). Its
  result becomes this leaf's own Exec result (rc/stdout/stdout_lines/stderr/
  stderr_lines), same as a real 'shell' task's Install would produce.
  ironstate.ps1's dispatch loop defers 'value's own '${{ }}' resolution (if
  given) until *after* this command has run - rather than the usual
  up-front per-leaf pass, which would only ever see an unresolved reference
  and omit the field - so 'value' can reference this same command's own
  result via bare 'rc'/'stdout'/'stdout_lines'/'stderr'/'stderr_lines' (the
  same self-reference convention 'failed_when' already uses). If 'value' is
  omitted, the fact is set directly to the command's trimmed stdout.

  Reuses Log.psm1's trick for a module with no real idempotency: Test
  reports "installed" exactly when 'state' is 'absent', so 'present'/'latest'
  always resolve to Install (fact gets (re)set) and 'absent' always resolves
  to Uninstall (fact gets unset) - a fact always fires when reached.

  Install runs 'shell' for real when present; otherwise it's a no-op beyond
  logging. Either way, the actual registry mutation (and any deferred
  'value' computation) happens in ironstate.ps1's dispatch loop, which reads
  this same 'name'/'value'/'shell' straight off the leaf's Item - regardless
  of '-Apply', since a fact has no real system side effect and dry-run
  previews of later 'when'/'${{ }}' references need it to behave the same as
  a real run.
#>

Set-StrictMode -Version Latest

Import-Module (Join-Path $PSScriptRoot '..\Common.psm1')
Import-Module (Join-Path $PSScriptRoot 'Shell.psm1')

function Get-FactHandler {
  [PSCustomObject]@{
    Test      = {
      param($Item)
      (Get-ItemState -Item $Item) -eq 'absent'
    }
    Describe  = {
      param($Item, $Action)
      $name = Get-Prop $Item 'name' (Get-Prop $item 'package' '<unnamed>')
      if ($Action -eq 'Uninstall') { "unset fact '$name'" }
      else {
        $shellSpec = Get-Prop $Item 'shell'
        if ($shellSpec) {
          $state = Get-ItemState -Item $Item
          $label = Get-ShellItemLabel -Item $shellSpec -State $state
          $config = Resolve-ShellStateConfig -Item $shellSpec -State $state
          "run shell '$label' via '$($config.HostSpec)' -> fact '$name'"
        } else {
          $value = Get-Prop $Item 'value'
          $preview = try { ConvertTo-Json -InputObject $value -Compress -Depth 5 } catch { [string] $value }
          "set fact '$name' = $preview"
        }
      }
    }
    Install   = {
      param($Item)
      $shellSpec = Get-Prop $Item 'shell'
      if ($shellSpec) {
        $state = Get-ItemState -Item $Item
        $config = Resolve-ShellStateConfig -Item $shellSpec -State $state
        return Invoke-ShellItem -Config $config -Label (Get-ShellItemLabel -Item $shellSpec -State $state)
      }
      Write-Verbose "fact '$(Get-Prop $Item 'name')' set"
    }
    Uninstall = {
      param($Item)
      Write-Verbose "fact '$(Get-Prop $Item 'name')' unset"
    }
  }
}

Export-ModuleMember -Function Get-FactHandler
