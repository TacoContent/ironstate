#!/usr/bin/env pwsh
<#
.SYNOPSIS
  Handler for the 'fact' module: sets an arbitrary named value for later
  tasks to reference.

.DESCRIPTION
  { name: <string>, value: <any YAML value>, state: present|absent }. 'value'
  can be a scalar, a list, or a nested mapping - stored as-is (it's already
  been through the per-leaf '${{ }}' template pass by the time this runs, so
  e.g. 'value: "${{ some_id }}"' resolves to that id's registered result).

  Reuses Log.psm1's trick for a module with no real idempotency: Test
  reports "installed" exactly when 'state' is 'absent', so 'present'/'latest'
  always resolve to Install (fact gets (re)set) and 'absent' always resolves
  to Uninstall (fact gets unset) - a fact always fires when reached.

  Install/Uninstall here are no-ops beyond logging: the actual registry
  mutation happens in ironstate.ps1's dispatch loop, which reads this same
  'name'/'value' straight off the leaf's Item - regardless of '-Apply',
  since a fact has no real system side effect and dry-run previews of later
  'when'/'${{ }}' references need it to behave the same as a real run.
#>

Set-StrictMode -Version Latest

Import-Module (Join-Path $PSScriptRoot '..\Common.psm1')

function Get-FactHandler {
  [PSCustomObject]@{
    Test      = {
      param($Item)
      (Get-ItemState -Item $Item) -eq 'absent'
    }
    Describe  = {
      param($Item, $Action)
      $name = Get-Prop $Item 'name' '<unnamed>'
      if ($Action -eq 'Uninstall') { "unset fact '$name'" }
      else {
        $value = Get-Prop $Item 'value'
        $preview = try { ConvertTo-Json -InputObject $value -Compress -Depth 5 } catch { [string] $value }
        "set fact '$name' = $preview"
      }
    }
    Install   = {
      param($Item)
      Write-Verbose "fact '$(Get-Prop $Item 'name')' set"
    }
    Uninstall = {
      param($Item)
      Write-Verbose "fact '$(Get-Prop $Item 'name')' unset"
    }
  }
}

Export-ModuleMember -Function Get-FactHandler
