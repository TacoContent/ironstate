#!/usr/bin/env pwsh
<#
.SYNOPSIS
  Declarative, Ansible-style task runner driven by site.yml.

.DESCRIPTION
  Reads a YAML document of tasks - either an explicit 'tasks: [...]' mapping
  or an implicit bare list - and reconciles each leaf action (winget, chocolatey,
  eget, copy, blockinfile, log, path, fact, ...) against its desired state:
  present, absent, or latest. A task can group several actions under one
  'name'/'tags'/'when'; 'when' conditions are evaluated against gathered
  host facts, user-defined 'vars', and a growing registry of 'id'-registered
  task results and 'fact' values (see below). An 'include' action pulls in
  another document's tasks from packages/<name>/main.yml (see the 'packages'
  folder and README.md) as if they were written inline.

  Ansible-style fact gathering: every 'fact' leaf in the whole (tag-filtered)
  tree runs first, in document order, before any other leaf - regardless of
  where in the file it's declared relative to whatever reads it. Because of
  this, a fact's own 'value'/'when' can only reference gathered facts, vars,
  and other facts gathered earlier in this same phase - never another task's
  'id', since no non-fact task has run yet. A fact needing a live command
  result computes it itself via its own embedded 'shell' instead.

  Each module's Test/Describe/Install/Uninstall behavior lives in its own
  PowerShell module under modules/Handlers/; the task tree itself is
  normalized/flattened by modules/Tasks.psm1. This script loads data, then
  walks the flattened, tag-filtered leaves *in order*, evaluating each
  leaf's 'when' and remaining '${{ }}' template references just before
  dispatching it - not grouped by module, and not all up front - so a leaf
  with 'id: foo' makes its result (changed/rc/stdout/stdout_lines/stderr/
  stderr_lines, Ansible-'register'-shaped) available to every later leaf's
  'when' (bare 'foo.rc') and '${{ }}' templates (bare '${{ foo }}'), and a
  'fact' action does the same for an arbitrary user-defined value.

.PARAMETER PackagesFile
  Path to the YAML file describing tasks. Defaults to ./site.yml.

.PARAMETER Apply
  Actually apply changes; installs and removals are performed. If omitted, the script runs in dry-run mode, only printing what would happen.

.PARAMETER Tags
  Restrict processing to tasks/actions carrying any of these tags. Tags
  cascade from a grouping task down to its nested actions. When omitted,
  every task/action is processed (subject to 'when').

.EXAMPLE
  ./ironstate.ps1 -Tags cli,security

.EXAMPLE
  ./ironstate.ps1 -Tags cli -Apply -Verbose
#>
[CmdletBinding()]
param(
  [Parameter(Mandatory = $false)]
  [string] $PackagesFile = "$PSScriptRoot/site.yml",

  [Parameter(Mandatory = $false)]
  [switch] $Apply,

  [Parameter(Mandatory = $false)]
  [string[]] $Tags
)

# Prefer pwsh (7+) over Windows PowerShell (5.1): relaunch under it if this
# session isn't already pwsh and pwsh is on PATH. Falls back to continuing
# under Windows PowerShell if pwsh isn't installed.
if ($PSVersionTable.PSVersion.Major -lt 7) {
  $pwshCommand = Get-Command pwsh -CommandType Application -ErrorAction SilentlyContinue | Select-Object -First 1
  if ($pwshCommand) {
    Write-Verbose "Relaunching under pwsh ($($pwshCommand.Source))..."
    $relaunchArgs = [System.Collections.Generic.List[string]]::new()
    $relaunchArgs.Add('-NoProfile')
    $relaunchArgs.Add('-File')
    $relaunchArgs.Add($PSCommandPath)
    $relaunchArgs.Add('-PackagesFile')
    $relaunchArgs.Add($PackagesFile)
    if ($Apply) { $relaunchArgs.Add('-Apply') }
    if ($Tags) { $relaunchArgs.Add('-Tags'); foreach ($tag in $Tags) { $relaunchArgs.Add($tag) } }
    if ($VerbosePreference -eq 'Continue') { $relaunchArgs.Add('-Verbose') }

    & $pwshCommand.Source @relaunchArgs
    exit $LASTEXITCODE
  }
  Write-Warning "pwsh not found on PATH; continuing under Windows PowerShell $($PSVersionTable.PSVersion). Install PowerShell 7+ (https://aka.ms/powershell) for the preferred runtime."
}

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

# Modules that aren't backed by an external CLI on PATH, so the usual
# "is the manager installed?" Get-Command check doesn't apply to them.
$script:NoCommandCheckModules = @('symlinks', 'zip', 'copy', 'shell', 'blockinfile', 'log', 'path', 'fact', 'registry', 'scheduled_task', 'file', 'template', 'assert', 'ssh_host_block')

# Modules whose task-tree name doesn't match the CLI binary it drives.
$script:ModuleCommandNames = @{ chocolatey = 'choco' }

# --- module loading -------------------------------------------------------

$modulesRoot = Join-Path $PSScriptRoot 'modules'
Import-Module (Join-Path $modulesRoot 'Common.psm1') -Force
Import-Module (Join-Path $modulesRoot 'Templates.psm1') -Force
Import-Module (Join-Path $modulesRoot 'Facts.psm1') -Force
Import-Module (Join-Path $modulesRoot 'Conditions.psm1') -Force
Import-Module (Join-Path $modulesRoot 'Tasks.psm1') -Force
Import-Module (Join-Path $modulesRoot 'Packages.psm1') -Force
Get-ChildItem -Path (Join-Path $modulesRoot 'Handlers') -Filter '*.psm1' | ForEach-Object {
  Import-Module $_.FullName -Force
}

function Get-PackageManagerHandlers {
  @{
    winget         = Get-WingetHandler
    chocolatey     = Get-ChocolateyHandler
    gem            = Get-RubyGemHandler
    pipx           = Get-PipxHandler
    npm            = Get-NpmHandler
    cargo          = Get-CargoHandler
    go             = Get-GoHandler
    eget           = Get-EgetHandler
    zip            = Get-ZipHandler
    symlinks       = Get-SymlinksHandler
    file           = Get-FileHandler
    copy           = Get-CopyHandler
    template       = Get-TemplateHandler
    shell          = Get-ShellHandler
    blockinfile    = Get-BlockInFileHandler
    ssh_host_block = Get-SshHostBlockHandler
    log            = Get-LogHandler
    path           = Get-PathHandler
    fact           = Get-FactHandler
    registry       = Get-RegistryHandler
    scheduled_task = Get-ScheduledTaskHandler
    assert         = Get-AssertHandler
  }
}

# --- execution --------------------------------------------------------

function Invoke-PackageItem {
  param(
    [Parameter(Mandatory)][string] $Module,
    [string] $Name,
    [Parameter(Mandatory)] $Item,
    [Parameter(Mandatory)][PSCustomObject] $Handler,
    [hashtable] $Context = @{},
    [switch] $Apply
  )

  $label = if ($Name) { $Name } else { Get-ItemLabel -Item $Item }
  $state = Get-ItemState -Item $Item
  $installed = & $Handler.Test $Item $Name $Context
  $action = Resolve-PackageAction -State $state -IsInstalled $installed

  # A handler's Install/Uninstall may return a { rc, stdout, stdout_lines,
  # stderr, stderr_lines } result hashtable (every CLI-backed handler and
  # 'shell' do) - 'shell' under the default 'pwsh' host can also merge extra
  # native-object properties alongside those (e.g. 'ProgramFilesDir' - see
  # Shell.psm1's Merge-ShellNativeResult), which must survive into '$exec'
  # below the same as the fixed fields, for '${{ <id>.ProgramFilesDir }}' /
  # a fact's embedded-shell 'value' to see them. Anything else Install/
  # Uninstall returns (or a thrown exception) is normalized below rather
  # than trusted, so pure-PowerShell handlers that return nothing
  # meaningful still produce a well-shaped result.
  $execResult = $null
  if ($action -eq 'Skip') {
    Write-Verbose "[$Module] $label - state=$state, installed=$installed -> skip"
  } else {
    $description = & $Handler.Describe $Item $action $Context
    if (-not $Apply) {
      Write-Host "[DryRun][$Module] $description"
    } else {
      Write-Host "[$Module] $description"
      try {
        $execResult = if ($action -eq 'Install') { & $Handler.Install $Item $Name $Context } else { & $Handler.Uninstall $Item $Name $Context }
      } catch {
        $message = $_.Exception.Message
        Write-Warning "[$Module] $label threw: $message"
        $execResult = @{ rc = 1; stdout = ''; stdout_lines = @(); stderr = $message; stderr_lines = @($message) }
      }
    }
  }

  $exec = Get-ZeroExecResult
  if ($execResult -is [System.Collections.IDictionary] -and $execResult.Contains('rc')) {
    # Every key, not just the fixed rc/stdout/.../stderr_lines set - carries
    # through any extra native-object properties a 'pwsh'-host 'shell'
    # merged in (see comment above).
    foreach ($key in $execResult.Keys) { $exec[$key] = $execResult[$key] }
  }

  [PSCustomObject]@{
    Module  = $Module
    Package = $label
    State   = $state
    Action  = $action
    Apply   = [bool] $Apply
    Exec    = $exec
  }
}

function Invoke-Tasks {
  # Dispatches tag-filtered leaves *sequentially*, in document order (not
  # grouped by module - matching Ansible's "tasks run in the order
  # written" behavior). Unlike a simple dispatch loop, this threads a
  # growing '$registry' (id-registered results + facts) forward: each
  # leaf's 'when' and remaining '${{ }}' references are resolved against
  # facts+vars+registry-so-far *immediately before* that leaf runs, so a
  # later leaf can see an earlier leaf's 'id'/'fact' - the whole reason
  # 'when' isn't evaluated any earlier, in Tasks.psm1's Expand-TaskTree.
  #
  # 'Registry'/'UserFacts'/'CommandAvailability' can be seeded by the
  # caller and are handed back on the returned object - this is what lets
  # the main flow below run this twice (a facts-gathering pass, then
  # everything else) as one continuous, threaded run instead of two
  # unrelated ones.
  param(
    [Parameter(Mandatory)] $Leaves,
    [Parameter(Mandatory)][hashtable] $Handlers,
    [Parameter(Mandatory)] $Facts,
    [Parameter(Mandatory)] $Vars,
    [hashtable] $Registry = @{},
    [hashtable] $UserFacts = @{},
    [hashtable] $CommandAvailability = @{},
    [switch] $Apply
  )

  $commandAvailability = $CommandAvailability
  $registry = $Registry
  $userFacts = $UserFacts
  $results = [System.Collections.Generic.List[object]]::new()
  $stoppedOnFailure = $false

  foreach ($leaf in @($Leaves)) {
    $module = $leaf.Module
    $label =
      if ($leaf.Name) { $leaf.Name }
      elseif (Get-Prop $leaf.Item 'name') { Get-Prop $leaf.Item 'name' }
      elseif (Get-Prop $leaf.Item 'package') { Get-Prop $leaf.Item 'package' }
      else { '<unnamed>' }

    $handler = $Handlers[$module]
    if (-not $handler) {
      Write-Warning "No handler registered for module '$module'; skipping."
      continue
    }

    if ($script:NoCommandCheckModules -notcontains $module) {
      $commandName = if ($script:ModuleCommandNames.Contains($module)) { $script:ModuleCommandNames[$module] } else { $module }
      if (-not $commandAvailability.Contains($module)) {
        $commandAvailability[$module] = [bool] (Get-Command $commandName -ErrorAction SilentlyContinue)
      }
      if (-not $commandAvailability[$module]) {
        Write-Warning "'$commandName' command not found on PATH; skipping."
        continue
      }
    }

    $flatContext = Merge-FlatContext -Facts $Facts -UserFacts $userFacts -PackageVars $leaf.PackageVars -PackageInputs $leaf.PackageInputs -PackagePackage $leaf.PackagePackage -Vars $Vars -Registry $registry

    # A 'fact' with an embedded 'shell' computes its value from that
    # command's own result, which doesn't exist yet at this point - defer
    # 'value's template resolution (if given) until after the command has
    # run, instead of resolving it here alongside everything else (which
    # would only ever see an unresolved reference and omit the field).
    $hasEmbeddedShell = ($module -eq 'fact') -and [bool] (Get-Prop $leaf.Item 'shell')
    $deferredFactValue = $null
    $hasDeferredFactValue = $false
    if ($hasEmbeddedShell -and $leaf.Item.Contains('value')) {
      $deferredFactValue = $leaf.Item['value']
      $hasDeferredFactValue = $true
      $leaf.Item.Remove('value')
    }

    # Strict (no '-Soft'): by now every remaining '${{ }}' reference should
    # be resolvable - facts/vars/package/inputs already were, earlier, in
    # the one-shot static pass(es). Anything still unresolved here is a
    # genuine error (typo, or an id/fact that never ran).
    $wrapper = @{ item = $leaf.Item }
    Resolve-TemplatesInPlace -Data $wrapper -Context $flatContext -PackageName $label
    $leaf.Item = $wrapper.item

    if (-not (Test-WhenClause -When $leaf.When -Context $flatContext)) {
      Write-Verbose "[$module] $label - when false -> skip"
      continue
    }

    # A fact's embedded 'shell' has no real system side effect (same
    # rationale as the fact registry mutation below) - always actually runs,
    # even without '-Apply', so dry-run previews of later 'when'/'${{ }}'
    # references see a real computed value instead of a zero-result stand-in.
    # 'assert' gets the same treatment for a different reason: it has no
    # system side effect at all - it *is* the check - so a dry run must
    # still actually evaluate it, or 'continue_on_error'/later 'when's would
    # never see a real pass/fail.
    $result = Invoke-PackageItem -Module $module -Name $leaf.Name -Item $leaf.Item -Handler $handler -Context $flatContext -Apply:($Apply -or $hasEmbeddedShell -or ($module -eq 'assert'))

    $changed = ($result.Action -ne 'Skip')
    $failed = ($result.Exec.rc -ne 0)
    if ($leaf.FailedWhen) {
      $failedWhenContext = $flatContext.Clone()
      foreach ($key in $result.Exec.Keys) { $failedWhenContext[$key] = $result.Exec[$key] }
      $failedWhenContext['changed'] = $changed
      $failed = Test-WhenClause -When $leaf.FailedWhen -Context $failedWhenContext
    }

    $result | Add-Member -NotePropertyName Failed -NotePropertyValue $failed
    $results.Add($result)

    if ($failed) {
      if ($leaf.ContinueOnError) {
        Write-Warning "[$module] $label failed (rc=$($result.Exec.rc)); continuing (continue_on_error)."
      } else {
        Write-Warning "[$module] $label failed (rc=$($result.Exec.rc)); stopping. Set continue_on_error: true to continue past this failure."
        $stoppedOnFailure = $true
        break
      }
    }

    if ($module -eq 'fact') {
      $factName = Get-Prop $leaf.Item 'name'
      if ($factName) {
        if ($result.Action -eq 'Uninstall') {
          $userFacts.Remove($factName)
        } elseif ($hasEmbeddedShell) {
          if ($hasDeferredFactValue) {
            # Resolve the deferred 'value' now that the embedded shell has
            # run, against facts/vars/id-registry plus this leaf's own bare
            # rc/stdout/stdout_lines/stderr/stderr_lines (self-reference,
            # same convention 'failed_when' uses above).
            $valueContext = $flatContext.Clone()
            foreach ($key in $result.Exec.Keys) { $valueContext[$key] = $result.Exec[$key] }
            $valueWrapper = @{ value = $deferredFactValue }
            Resolve-TemplatesInPlace -Data $valueWrapper -Context $valueContext -PackageName $label
            $userFacts[$factName] = $valueWrapper.value
          } else {
            $userFacts[$factName] = ([string] $result.Exec.stdout).Trim()
          }
        } else {
          $userFacts[$factName] = Get-Prop $leaf.Item 'value'
        }
      }
    }

    if ($leaf.Id) {
      $registered = @{
        changed      = $changed
        failed       = $failed
        rc           = $result.Exec.rc
        stdout       = $result.Exec.stdout
        stdout_lines = $result.Exec.stdout_lines
        stderr       = $result.Exec.stderr
        stderr_lines = $result.Exec.stderr_lines
      }
      # Any extra native-object properties a 'pwsh'-host 'shell' merged in
      # (e.g. 'ProgramFilesDir') ride along too, never overwriting the
      # reserved fields above - see Shell.psm1's Merge-ShellNativeResult.
      foreach ($key in $result.Exec.Keys) {
        if (-not $registered.Contains($key)) { $registered[$key] = $result.Exec[$key] }
      }

      if ($leaf.Looped) {
        # A looped task's 'id' is the same string on every iteration - naively
        # overwriting $registry[$leaf.Id] each time would silently keep only
        # the last iteration's result (Ansible avoids exactly this by
        # aggregating a looped+registered task into '.results[]'). The
        # convenience flat fields (rc/stdout/...) still reflect the most
        # recent iteration; '.results[N]' holds every iteration in order.
        $priorResults =
          if ($registry.Contains($leaf.Id) -and $registry[$leaf.Id].Contains('results')) { @($registry[$leaf.Id]['results']) }
          else { @() }
        $allResults = [System.Collections.Generic.List[object]]::new()
        foreach ($r in $priorResults) { $allResults.Add($r) }
        $allResults.Add($registered)

        $flatView = @{}
        foreach ($key in $registered.Keys) { $flatView[$key] = $registered[$key] }
        $flatView['results'] = $allResults.ToArray()
        $registry[$leaf.Id] = $flatView
      } else {
        $registry[$leaf.Id] = $registered
      }
    }
  }

  return [PSCustomObject]@{
    Results             = $results.ToArray()
    StoppedOnFailure    = $stoppedOnFailure
    Registry            = $registry
    UserFacts           = $userFacts
    CommandAvailability = $commandAvailability
  }
}

# --- main ----------------------------------------------------------------

$repoRoot = Split-Path (Split-Path $PSScriptRoot -Parent) -Parent
Import-EnvFile -Path (Join-Path $repoRoot '.env')
Import-EnvFile -Path (Join-Path $repoRoot '.secrets')

Initialize-YamlModule

$facts = Get-InstallFacts
Write-Verbose "Facts: $(($facts.Keys | Sort-Object | ForEach-Object { "$_=$($facts[$_])" }) -join ', ')"

$resolvedPackagesFile = Resolve-UserPath $PackagesFile
$data = Import-PackagesHierarchy -BaseFile $resolvedPackagesFile

$vars = Get-Prop $data 'vars' @{}
# '-Soft': resolves facts/vars/package/inputs now; leaves any bare id/fact
# reference (e.g. '${{ bar }}') untouched for Invoke-Tasks's per-leaf pass,
# once that registry actually exists.
Resolve-TemplatesInPlace -Data $data -Context @{ facts = $facts; vars = $vars } -PackageName 'site' -Soft

$handlers = Get-PackageManagerHandlers
$moduleNames = @($handlers.Keys)

$taskList = Get-TaskList -Data $data
# (Join-Path $PSScriptRoot 'packages')
$leaves = Expand-TaskTree -Tasks $taskList -ModuleNames $moduleNames -PackagesRoot $PSScriptRoot -Facts $facts -Vars $vars
$filteredLeaves = @($leaves | Where-Object { Test-TagsMatch -Tags $_.Tags -Filter $Tags })

# Ansible-style "gather facts" phase: every 'fact' leaf across the whole
# (tag-filtered) tree runs first, in its own document order, before any
# other leaf - so a 'when'/'${{ }}' reference to a user-defined fact never
# depends on where in the file that fact happens to be declared relative
# to whatever reads it. This also means a fact's own 'value'/'when' can
# only see facts/vars (gathered facts and this same phase's own
# user-defined facts so far) - never another task's 'id', since no
# non-fact task has run yet at this point. A fact needing a live command
# result has to compute it itself via its own embedded 'shell' (see
# Handlers/Fact.psm1) rather than reading a separately-'id'd task.
$factLeaves = @($filteredLeaves | Where-Object { $_.Module -eq 'fact' })
$otherLeaves = @($filteredLeaves | Where-Object { $_.Module -ne 'fact' })

$factRun = Invoke-Tasks -Leaves $factLeaves -Handlers $handlers -Facts $facts -Vars $vars -Apply:$Apply

$allResults = [System.Collections.Generic.List[object]]::new()
foreach ($r in $factRun.Results) { $allResults.Add($r) }

$stoppedOnFailure = $factRun.StoppedOnFailure
if (-not $stoppedOnFailure) {
  $run = Invoke-Tasks -Leaves $otherLeaves -Handlers $handlers -Facts $facts -Vars $vars -Apply:$Apply -Registry $factRun.Registry -UserFacts $factRun.UserFacts -CommandAvailability $factRun.CommandAvailability
  foreach ($r in $run.Results) { $allResults.Add($r) }
  $stoppedOnFailure = $run.StoppedOnFailure
}

$allResults | Format-Table -Property Module, Package, State, Action, Failed -AutoSize

if ($stoppedOnFailure) { exit 1 }
