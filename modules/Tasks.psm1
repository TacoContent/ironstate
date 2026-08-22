#!/usr/bin/env pwsh
<#
.SYNOPSIS
  Task/action tree normalization and flattening for the ironstate.ps1 runner.

.DESCRIPTION
  A loaded document is either an explicit '{ tasks: [...] }' mapping or an
  implicit bare list (Get-TaskList normalizes both to a plain list). From
  there, every item in that list is classified independently, so "a list of
  tasks" and "a list of bare actions" are the same mechanism:
    - an item with an 'actions' key is a grouping task whose 'name'/'tags'/
      'when' scope the whole subtree (recursed through the same rule, to
      arbitrary depth);
    - an item with an 'include' key pulls in another document's task list
      (packages/<name>/main.yml) via Packages.psm1's Import-IncludedPackage,
      and is otherwise treated exactly like 'actions' - a subtree, just
      sourced from a file instead of written inline;
    - anything else is a leaf - its one recognized module key (e.g.
      'winget', 'copy', 'log') is dispatched directly, exactly like a
      group's would be.

  Expand-TaskTree walks that tree once, accumulating 'tags' top-down (a
  leaf's effective tags are the union of its own tags with every
  ancestor's) and accumulating 'when' top-down into a flat list (a leaf's
  own 'when' plus every ancestor's, AND'd - Ansible block+task semantics),
  producing a flat list of leaves ready for dispatch. Unlike 'tags', 'when'
  is NOT evaluated here: a leaf's 'when' may reference an earlier leaf's
  'id'-registered result or a 'fact', neither of which exist yet at flatten
  time (nothing has executed), so evaluation is deferred to ironstate.ps1's
  per-leaf dispatch loop, which threads a growing registry forward in
  document order. The inner module dict (e.g. '{package, source, state}')
  passed to each leaf is unchanged from today's per-group item shape, so no
  existing Handler needs to change.

  Looping ('with'/'items'): a task carrying either key is materialized once
  per loop value *before* anything else about it is looked at - name, tags,
  when, id, its module dict, or a nested 'actions'/'include' - each copy
  gets '${{ item }}'/'${{ item.<key> }}' resolved against that one value,
  then re-enters this same function as an ordinary (now loop-free) task.
  'items' is a list (one materialized copy per entry, Ansible 'loop'-style);
  'with' is a single value (exactly one copy - "use this as a reference"
  without iterating). Both share the one 'item' template context; 'items'
  wins if a task has both. Every leaf produced this way carries 'Looped =
  $true' (sticky through nested 'actions'/'include', so looping a whole
  block marks every leaf inside it too) - ironstate.ps1's dispatch loop uses
  this to know an 'id' shared across iterations should accumulate into a
  '.results[]' array rather than the last iteration silently overwriting
  the others.

  A loop nested inside another loop's 'actions' rebinds 'item' to its own
  value, which would otherwise make the outer value unreachable - the
  enclosing loop's context is exposed instead as '${{ parent.item }}' /
  '${{ parent.item.<key> }}', chaining '${{ parent.parent.item }}' etc. for
  further ancestors (see '-ParentItemContext' below).
#>

Set-StrictMode -Version Latest

Import-Module (Join-Path $PSScriptRoot 'Common.psm1')
Import-Module (Join-Path $PSScriptRoot 'Templates.psm1')
Import-Module (Join-Path $PSScriptRoot 'Packages.psm1')

function Get-TaskList {
  # Normalizes a loaded document's root into a plain task list: the explicit
  # '{ tasks: [...] }' form, or the document itself when it's already a bare
  # list (implicit form).
  param($Data)

  if ($Data -is [System.Collections.IDictionary]) {
    if ($Data.Contains('tasks')) { return ,@($Data['tasks']) }
    return ,@()
  }
  if ($Data -is [System.Collections.IList]) { return ,@($Data) }

  throw "Document root must be a '{ tasks: [...] }' mapping or a bare list of tasks/actions."
}

function Expand-TaskTree {
  # Flattens nested tasks/actions/includes into leaves:
  # [PSCustomObject]@{ Name; Tags; When; Id; Looped; Module; Item; PackageVars }.
  # 'When' is the accumulated (unevaluated) list of ancestor + own condition
  # strings - see the module docstring for why evaluation is deferred to
  # the caller. 'PackageVars' is the leaf's immediately-enclosing package's
  # own 'vars:' block (see the '-PackageVars' param below), for
  # ironstate.ps1's dispatch loop to layer into that leaf's flat context.
  # '-PackagesRoot' is only needed when the tree actually uses 'include' (a
  # task with 'include' but no PackagesRoot configured warns and is
  # skipped, rather than throwing).
  param(
    [Parameter(Mandatory)] $Tasks,
    [Parameter(Mandatory)][string[]] $ModuleNames,
    [string] $PackagesRoot,
    $Facts = @{},
    $Vars = @{},
    # The immediately-enclosing package's own 'vars:' block (see
    # Packages.psm1's Import-IncludedPackage), stamped onto every leaf so
    # ironstate.ps1's dispatch loop can layer it into that leaf's flat
    # context (Common.psm1's Merge-FlatContext). Reset - not merged - at
    # each 'include:' branch below, so a leaf only ever sees its *own*
    # package's local vars, never an ancestor package's.
    $PackageVars = @{},
    [string[]] $ParentTags = @(),
    [string[]] $ParentWhen = @(),
    [bool] $ParentLooped = $false,
    # The immediately-enclosing loop's own '{ item; parent }' template
    # context (see the module docstring's "Looping" section) - $null outside
    # any loop. Threaded down so a nested 'with'/'items' can expose its
    # enclosing loop's value as '${{ parent.item }}' / '${{ parent.item.<key> }}',
    # chaining '${{ parent.parent.item }}' etc. for further ancestors. Reset
    # to $null at an 'include:' branch, matching 'PackageVars' isolation
    # below - an included package doesn't implicitly see whatever loop
    # variable happens to be in scope wherever it was included from.
    $ParentItemContext = $null
  )

  $results = [System.Collections.Generic.List[object]]::new()

  foreach ($item in @($Tasks)) {
    if ($null -eq $item) { continue }

    if ($item.Contains('with') -or $item.Contains('items')) {
      $loopLabel = Get-Prop $item 'name' (Get-Prop $item 'package' '<unnamed>')
      if ($item.Contains('with') -and $item.Contains('items')) {
        Write-Warning "Task '$loopLabel' has both 'with' and 'items'; using 'items' and ignoring 'with'."
      }

      # Built via .Add(), not the ', $x' comma-wrap idiom used elsewhere in
      # this codebase: when $x is itself enumerable (a YAML list under
      # 'with'), ', $x' assigned from an if/else *expression* does not
      # reliably survive as a single element - PowerShell unrolls it one
      # level on the way out, so 'with' would wrongly iterate instead of
      # treating the whole list as one opaque reference value. Verified
      # empirically. .Add() takes exactly one argument regardless of that
      # argument's own shape, so there's no enumeration ambiguity.
      $loopValues = [System.Collections.Generic.List[object]]::new()
      if ($item.Contains('items')) {
        foreach ($v in @(Get-Prop $item 'items' @())) { $loopValues.Add($v) }
      } else {
        $loopValues.Add((Get-Prop $item 'with'))
      }

      $template = Copy-DeepData -Data $item
      $template.Remove('with')
      $template.Remove('items')

      foreach ($loopValue in $loopValues) {
        $materialized = Copy-DeepData -Data $template
        $wrapper = @{ task = $materialized }
        $itemContext = @{ item = $loopValue }
        if ($null -ne $ParentItemContext) { $itemContext['parent'] = $ParentItemContext }
        Resolve-TemplatesInPlace -Data $wrapper -Context $itemContext -PackageName $loopLabel -Soft -BoundaryKeys @('items', 'with')
        $children = Expand-TaskTree -Tasks @($wrapper.task) -ModuleNames $ModuleNames -PackagesRoot $PackagesRoot -Facts $Facts -Vars $Vars -PackageVars $PackageVars -ParentTags $ParentTags -ParentWhen $ParentWhen -ParentLooped $true -ParentItemContext $itemContext
        foreach ($child in $children) { $results.Add($child) }
      }
      continue
    }

    $effectiveTags = @($ParentTags + @(Get-Prop $item 'tags' @()) | Select-Object -Unique)
    $effectiveWhen = @($ParentWhen) + @(Get-Prop $item 'when' @())
    $label = Get-Prop $item 'name' (Get-Prop $item 'package' '<unnamed>')

    if ($item.Contains('actions')) {
      if ($item.Contains('id')) { Write-Warning "Task '$label' has an 'id' but is a grouping task (has 'actions'); 'id' is only supported on leaf actions - ignoring." }
      $children = Expand-TaskTree -Tasks (Get-Prop $item 'actions' @()) -ModuleNames $ModuleNames -PackagesRoot $PackagesRoot -Facts $Facts -Vars $Vars -PackageVars $PackageVars -ParentTags $effectiveTags -ParentWhen $effectiveWhen -ParentLooped $ParentLooped -ParentItemContext $ParentItemContext
      foreach ($child in $children) { $results.Add($child) }
      continue
    }

    if ($item.Contains('include')) {
      if ($item.Contains('id')) { Write-Warning "Task '$label' has an 'id' but is an 'include'; 'id' is only supported on leaf actions - ignoring." }
      if (-not $PackagesRoot) {
        Write-Warning "Task '$label' has an 'include' but no PackagesRoot was configured; skipping."
        continue
      }

      $pkgData = Import-IncludedPackage -IncludeSpec $item['include'] -PackagesRoot $PackagesRoot -Facts $Facts -Vars $Vars
      if ($null -eq $pkgData) { continue }

      # Fresh per include - a package's own local vars don't inherit from
      # (or merge with) whichever package, if any, is including it.
      $childPackageVars = Get-Prop $pkgData 'vars' @{}
      $includedTasks = Get-TaskList -Data $pkgData
      $children = Expand-TaskTree -Tasks $includedTasks -ModuleNames $ModuleNames -PackagesRoot $PackagesRoot -Facts $Facts -Vars $Vars -PackageVars $childPackageVars -ParentTags $effectiveTags -ParentWhen $effectiveWhen -ParentLooped $ParentLooped
      foreach ($child in $children) { $results.Add($child) }
      continue
    }

    $moduleKeys = @($item.Keys | Where-Object { $ModuleNames -contains $_ })

    if ($moduleKeys.Count -eq 0) {
      Write-Warning "Task '$label' has no recognized module key (expected one of: $($ModuleNames -join ', ')); skipping."
      continue
    }
    if ($moduleKeys.Count -gt 1) {
      Write-Warning "Task '$label' has multiple module keys ($($moduleKeys -join ', ')); using '$($moduleKeys[0])' and ignoring the rest."
    }

    $moduleName = $moduleKeys[0]
    $results.Add([PSCustomObject]@{
      Name            = Get-Prop $item 'name' $null
      Tags            = $effectiveTags
      When            = $effectiveWhen
      Id              = Get-Prop $item 'id' $null
      FailedWhen      = Get-Prop $item 'failed_when' $null
      ContinueOnError = Get-Prop $item 'continue_on_error' $null
      Looped          = $ParentLooped
      PackageVars     = $PackageVars
      Module          = $moduleName
      Item            = $item[$moduleName]
    })
  }

  # ',' prevents PowerShell from unrolling this across the function return
  # boundary (a 1-leaf result would otherwise arrive at the caller as a bare
  # PSCustomObject, and a 0-leaf result as $null - see the same note in
  # Templates.psm1's Expand-TemplateNode).
  return ,$results.ToArray()
}

Export-ModuleMember -Function Get-TaskList, Expand-TaskTree
