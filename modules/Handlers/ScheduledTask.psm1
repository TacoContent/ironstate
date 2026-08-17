#!/usr/bin/env pwsh
<#
.SYNOPSIS
  Handler for the 'scheduled_task' module: registers/updates/removes a
  Windows Task Scheduler task.

.DESCRIPTION
  { name, path, description, enabled, actions: [...], triggers: [...],
  principal: {...}, settings: {...}, state }. 'name'/'path' identify the task
  (TaskName/TaskPath, path defaults to the root folder '\' and is normalized
  to always start/end with '\'). Built entirely on the built-in ScheduledTasks
  module (Get/New/Register/Set/Unregister/Enable/Disable-ScheduledTask) - no
  functional gap turned up that needed an schtasks.exe fallback, so none is
  used here.

  'actions' (required unless 'state: absent') is a list of
  { execute, arguments, working_directory } - one per Task Scheduler action,
  in order. Only plain "run this executable" actions are supported (not
  COM-handler/email actions).

  'triggers' (optional - omit for a manual/on-demand-only task) is a list of
  { type, ... }, 'type' one of:
    - logon:   { user_id?, delay?, random_delay? }             (any user if user_id omitted)
    - startup: { delay?, random_delay? }
    - once:    { at, repetition_interval?, repetition_duration?, random_delay? }
    - daily:   { at, days_interval?, repetition_interval?, repetition_duration?, random_delay? }
    - weekly:  { at, days_of_week, weeks_interval?, repetition_interval?, repetition_duration?, random_delay? }

  Registering a task with a 'logon' or 'startup' trigger fails with "Access
  is denied" unless ironstate.ps1 is running elevated (Run as Administrator) -
  verified empirically against both Register-ScheduledTask and raw
  schtasks.exe, so it's a genuine Task Scheduler privilege requirement, not a
  gap either tool can route around. 'once'/'daily'/'weekly' triggers, and a
  task with no triggers at all, register fine as a standard user.
  'at' is a date/time string (full 'yyyy-MM-ddTHH:mm:ss' for 'once', just the
  time-of-day matters for 'daily'/'weekly'). 'delay'/'random_delay'/
  'repetition_interval'/'repetition_duration' accept either an ISO 8601
  duration ('PT30S', 'P1D') or a plain .NET TimeSpan string ('00:00:30',
  '1.00:00:00'). 'days_of_week' is a list of day names (Monday, Tuesday, ...).

  'principal' (optional - omit to run as the current user with standard
  rights) is { user_id | group_id, logon_type?, run_level? }. 'logon_type' is
  one of None/Password/S4U/Interactive/Group/ServiceAccount/
  InteractiveOrPassword; 'run_level' is Limited (default) or Highest.
  'logon_type: Password' additionally requires 'user_id' and 'password_env'
  (the *name* of an environment variable - populated via this repo's own
  .env/.secrets loading, see Packages.psm1's Import-EnvFile - holding the
  actual password; never write a plaintext password into YAML). Note: a
  stored task password can't be read back, so idempotency can't detect a
  drifted password - only 'state: latest' is guaranteed to re-apply one.

  'settings' (optional, partial-declare like the 'registry' module's
  'values': only the keys you list are managed/compared, everything else is
  left at Task Scheduler's own defaults) maps snake_case keys directly onto
  ScheduledTaskSettingsSet properties: disallow_start_if_on_batteries,
  start_when_available, hidden, wake_to_run, allow_hard_terminate,
  run_only_if_network_available, run_only_if_idle (bools);
  multiple_instances (IgnoreNew/Parallel/Queue/StopExisting); restart_count
  (int - Task Scheduler silently ignores this unless 'restart_interval' is
  also set, so declare both together); execution_time_limit/
  restart_interval/delete_expired_task_after (duration strings, same formats
  as triggers - 'PT0S' means "no limit" for execution_time_limit).

  'enabled' (default true) is handled via Enable-ScheduledTask/
  Disable-ScheduledTask after every (re)registration, not folded into
  'settings' - it's the one thing managed through the dedicated cmdlets
  rather than a raw settings-object property.

  Test is state-aware like Registry.psm1: for 'present'/'latest' the task
  must exist AND every declared field (actions, triggers, declared principal
  fields, declared settings fields, description, enabled) must match, so a
  drifted task gets corrected (re-registered wholesale via
  Register-ScheduledTask -Force), not skipped. Actions/triggers are compared
  positionally (same order as written) - an existing task carrying an extra
  or foreign trigger/action (one this handler didn't put there, or a
  type it doesn't model, e.g. an event/idle/registration trigger) also
  counts as a mismatch, so re-registering replaces it with exactly the
  declared set (declarative/authoritative, matching how 'registry' corrects
  a wrong-typed value rather than leaving it alongside the right one).
  'absent' only needs the task to exist, regardless of any of the above.
#>

Set-StrictMode -Version Latest

Import-Module (Join-Path $PSScriptRoot '..\Common.psm1')

# --- duration/time helpers -------------------------------------------------

function ConvertTo-TaskTimeSpan {
  # Accepts an ISO 8601 duration ('PT30S', 'P1D') or a plain .NET TimeSpan
  # string ('00:00:30', '1.00:00:00'). $null/'' -> $null (means "not set").
  param($Value)
  if ($null -eq $Value -or "$Value" -eq '') { return $null }
  if ($Value -is [TimeSpan]) { return $Value }
  $s = "$Value".Trim()
  if ($s.StartsWith('P')) { return [System.Xml.XmlConvert]::ToTimeSpan($s) }
  return [TimeSpan]::Parse($s)
}

function ConvertTo-Iso8601Duration {
  # Canonical form Task Scheduler itself stores/returns (e.g. 'PT30S') - used
  # whenever a duration has to be assigned directly onto a CIM property that
  # has no dedicated cmdlet parameter (Delay on logon/startup triggers).
  param($Value)
  $ts = ConvertTo-TaskTimeSpan $Value
  if ($null -eq $ts) { return $null }
  return [System.Xml.XmlConvert]::ToString($ts)
}

function ConvertTo-TaskDateTime {
  param([Parameter(Mandatory)][string] $Value)
  return [DateTime]::Parse($Value, [System.Globalization.CultureInfo]::InvariantCulture)
}

# Task Scheduler's DaysOfWeek bitmask (distinct from .NET's [DayOfWeek] 0-6
# ordinal values) - verified empirically against a live registered trigger.
$script:DayBits = [ordered]@{ Sunday = 1; Monday = 2; Tuesday = 4; Wednesday = 8; Thursday = 16; Friday = 32; Saturday = 64 }

function ConvertTo-DayOfWeekArray {
  param($Names)
  return @(@($Names) | ForEach-Object { [System.DayOfWeek][Enum]::Parse([System.DayOfWeek], "$_", $true) })
}

function ConvertFrom-DaysOfWeekBitmask {
  param([uint32] $Bitmask)
  return @($script:DayBits.Keys | Where-Object { ($Bitmask -band $script:DayBits[$_]) -ne 0 } | Sort-Object)
}

function Resolve-TaskFolderPath {
  param([string] $Path = '\')
  if ([string]::IsNullOrEmpty($Path)) { return '\' }
  $normalized = $Path.Trim()
  if (-not $normalized.StartsWith('\')) { $normalized = '\' + $normalized }
  if (-not $normalized.EndsWith('\')) { $normalized = $normalized + '\' }
  return $normalized
}

# --- actions ----------------------------------------------------------------

function Get-DesiredActionSpecs {
  param($Item)
  return @(@(Get-Prop $Item 'actions' @()) | ForEach-Object {
    [ordered]@{
      Execute          = Get-Prop $_ 'execute'
      Arguments        = [string] (Get-Prop $_ 'arguments' '')
      WorkingDirectory = [string] (Get-Prop $_ 'working_directory' '')
    }
  })
}

function Get-ExistingActionSpecs {
  param($Task)
  return @(@($Task.Actions) | ForEach-Object {
    [ordered]@{
      Execute          = [string] $_.Execute
      Arguments        = [string] $_.Arguments
      WorkingDirectory = [string] $_.WorkingDirectory
    }
  })
}

function Test-TaskActionListsMatch {
  param($Desired, $Existing)
  if (@($Desired).Count -ne @($Existing).Count) { return $false }
  for ($i = 0; $i -lt @($Desired).Count; $i++) {
    $d = $Desired[$i]; $e = $Existing[$i]
    if ($d.Execute -ne $e.Execute -or $d.Arguments -ne $e.Arguments -or $d.WorkingDirectory -ne $e.WorkingDirectory) { return $false }
  }
  return $true
}

# --- triggers -----------------------------------------------------------

function New-EmptyTriggerSpec {
  [ordered]@{
    Type = $null; At = $null; DaysInterval = $null; WeeksInterval = $null; DaysOfWeek = @()
    UserId = $null; Delay = $null; RandomDelay = $null; RepetitionInterval = $null; RepetitionDuration = $null
  }
}

function ConvertTo-DesiredTriggerSpec {
  param($Trigger)
  $type = "$(Get-Prop $Trigger 'type')".ToLowerInvariant()
  $spec = New-EmptyTriggerSpec
  $spec.Type = $type

  switch ($type) {
    'logon' {
      $spec.UserId = Get-Prop $Trigger 'user_id'
      $spec.Delay = ConvertTo-TaskTimeSpan (Get-Prop $Trigger 'delay')
      $spec.RandomDelay = ConvertTo-TaskTimeSpan (Get-Prop $Trigger 'random_delay')
    }
    'startup' {
      $spec.Delay = ConvertTo-TaskTimeSpan (Get-Prop $Trigger 'delay')
      $spec.RandomDelay = ConvertTo-TaskTimeSpan (Get-Prop $Trigger 'random_delay')
    }
    'once' {
      $spec.At = ConvertTo-TaskDateTime (Get-Prop $Trigger 'at')
      $spec.RandomDelay = ConvertTo-TaskTimeSpan (Get-Prop $Trigger 'random_delay')
      $spec.RepetitionInterval = ConvertTo-TaskTimeSpan (Get-Prop $Trigger 'repetition_interval')
      $spec.RepetitionDuration = ConvertTo-TaskTimeSpan (Get-Prop $Trigger 'repetition_duration')
    }
    'daily' {
      $spec.At = (ConvertTo-TaskDateTime (Get-Prop $Trigger 'at')).TimeOfDay
      $spec.DaysInterval = [int] (Get-Prop $Trigger 'days_interval' 1)
      $spec.RandomDelay = ConvertTo-TaskTimeSpan (Get-Prop $Trigger 'random_delay')
      $spec.RepetitionInterval = ConvertTo-TaskTimeSpan (Get-Prop $Trigger 'repetition_interval')
      $spec.RepetitionDuration = ConvertTo-TaskTimeSpan (Get-Prop $Trigger 'repetition_duration')
    }
    'weekly' {
      $spec.At = (ConvertTo-TaskDateTime (Get-Prop $Trigger 'at')).TimeOfDay
      $spec.DaysOfWeek = @(@(Get-Prop $Trigger 'days_of_week' @()) | ForEach-Object { "$_" } | Sort-Object)
      $spec.WeeksInterval = [int] (Get-Prop $Trigger 'weeks_interval' 1)
      $spec.RandomDelay = ConvertTo-TaskTimeSpan (Get-Prop $Trigger 'random_delay')
      $spec.RepetitionInterval = ConvertTo-TaskTimeSpan (Get-Prop $Trigger 'repetition_interval')
      $spec.RepetitionDuration = ConvertTo-TaskTimeSpan (Get-Prop $Trigger 'repetition_duration')
    }
    default { throw "Unknown scheduled_task trigger type '$type' (expected one of: logon, startup, once, daily, weekly)" }
  }
  return $spec
}

function ConvertTo-ExistingTriggerSpec {
  # Returns $null for a trigger type this handler doesn't model (event/idle/
  # registration/session-state triggers) - callers treat that as "foreign
  # object present", i.e. a mismatch to be corrected.
  param($ExistingTrigger)
  $spec = New-EmptyTriggerSpec

  switch ($ExistingTrigger.CimClass.CimClassName) {
    'MSFT_TaskLogonTrigger' {
      $spec.Type = 'logon'
      $spec.UserId = $ExistingTrigger.UserId
      $spec.Delay = ConvertTo-TaskTimeSpan $ExistingTrigger.Delay
    }
    'MSFT_TaskBootTrigger' {
      $spec.Type = 'startup'
      $spec.Delay = ConvertTo-TaskTimeSpan $ExistingTrigger.Delay
    }
    'MSFT_TaskTimeTrigger' {
      $spec.Type = 'once'
      $spec.At = [DateTime]::Parse($ExistingTrigger.StartBoundary, [System.Globalization.CultureInfo]::InvariantCulture)
      $spec.RandomDelay = ConvertTo-TaskTimeSpan $ExistingTrigger.RandomDelay
      if ($ExistingTrigger.Repetition) {
        $spec.RepetitionInterval = ConvertTo-TaskTimeSpan $ExistingTrigger.Repetition.Interval
        $spec.RepetitionDuration = ConvertTo-TaskTimeSpan $ExistingTrigger.Repetition.Duration
      }
    }
    'MSFT_TaskDailyTrigger' {
      $spec.Type = 'daily'
      $spec.At = ([DateTime]::Parse($ExistingTrigger.StartBoundary, [System.Globalization.CultureInfo]::InvariantCulture)).TimeOfDay
      $spec.DaysInterval = [int] $ExistingTrigger.DaysInterval
      $spec.RandomDelay = ConvertTo-TaskTimeSpan $ExistingTrigger.RandomDelay
      if ($ExistingTrigger.Repetition) {
        $spec.RepetitionInterval = ConvertTo-TaskTimeSpan $ExistingTrigger.Repetition.Interval
        $spec.RepetitionDuration = ConvertTo-TaskTimeSpan $ExistingTrigger.Repetition.Duration
      }
    }
    'MSFT_TaskWeeklyTrigger' {
      $spec.Type = 'weekly'
      $spec.At = ([DateTime]::Parse($ExistingTrigger.StartBoundary, [System.Globalization.CultureInfo]::InvariantCulture)).TimeOfDay
      $spec.DaysOfWeek = ConvertFrom-DaysOfWeekBitmask -Bitmask ([uint32] $ExistingTrigger.DaysOfWeek)
      $spec.WeeksInterval = [int] $ExistingTrigger.WeeksInterval
      $spec.RandomDelay = ConvertTo-TaskTimeSpan $ExistingTrigger.RandomDelay
      if ($ExistingTrigger.Repetition) {
        $spec.RepetitionInterval = ConvertTo-TaskTimeSpan $ExistingTrigger.Repetition.Interval
        $spec.RepetitionDuration = ConvertTo-TaskTimeSpan $ExistingTrigger.Repetition.Duration
      }
    }
    default { return $null }
  }
  return $spec
}

function Test-NormalizedTriggersEqual {
  param($A, $B)
  foreach ($key in @('Type', 'At', 'DaysInterval', 'WeeksInterval', 'DaysOfWeek', 'UserId', 'Delay', 'RandomDelay', 'RepetitionInterval', 'RepetitionDuration')) {
    $av = $A[$key]; $bv = $B[$key]
    switch ($key) {
      'DaysOfWeek' { if ((@($av) -join ',') -ne (@($bv) -join ',')) { return $false } }
      { $_ -in @('Delay', 'RandomDelay', 'RepetitionInterval', 'RepetitionDuration') } {
        $at = if ($null -ne $av) { $av } else { [TimeSpan]::Zero }
        $bt = if ($null -ne $bv) { $bv } else { [TimeSpan]::Zero }
        if ($at -ne $bt) { return $false }
      }
      'At' {
        if (($null -eq $av) -ne ($null -eq $bv)) { return $false }
        if ($null -ne $av -and $av -ne $bv) { return $false }
      }
      default { if ("$av" -ne "$bv") { return $false } }
    }
  }
  return $true
}

function Test-TaskTriggerListsMatch {
  param($Desired, $Existing)
  if (@($Desired).Count -ne @($Existing).Count) { return $false }
  if (@($Existing) -contains $null) { return $false }
  for ($i = 0; $i -lt @($Desired).Count; $i++) {
    if (-not (Test-NormalizedTriggersEqual -A $Desired[$i] -B $Existing[$i])) { return $false }
  }
  return $true
}

function New-TaskRepetitionPattern {
  # Daily/Weekly triggers have no -RepetitionInterval/-RepetitionDuration
  # cmdlet parameter (only 'Once' does) - the CIM Repetition sub-object has
  # to be built and assigned directly. Verified empirically.
  param([TimeSpan] $Interval, [TimeSpan] $Duration)
  return New-CimInstance -ClassName MSFT_TaskRepetitionPattern -Namespace 'Root/Microsoft/Windows/TaskScheduler' -ClientOnly -Property @{
    Interval          = ConvertTo-Iso8601Duration $Interval
    Duration          = if ($Duration) { ConvertTo-Iso8601Duration $Duration } else { '' }
    StopAtDurationEnd = $false
  }
}

function New-TaskTriggerFromSpec {
  param($Trigger)
  $type = "$(Get-Prop $Trigger 'type')".ToLowerInvariant()

  switch ($type) {
    'logon' {
      $args = @{ AtLogOn = $true }
      $userId = Get-Prop $Trigger 'user_id'
      if ($userId) { $args['User'] = $userId }
      $randomDelay = ConvertTo-TaskTimeSpan (Get-Prop $Trigger 'random_delay')
      if ($randomDelay) { $args['RandomDelay'] = $randomDelay }
      $t = New-ScheduledTaskTrigger @args
      $delay = ConvertTo-Iso8601Duration (Get-Prop $Trigger 'delay')
      if ($delay) { $t.Delay = $delay }
      return $t
    }
    'startup' {
      $args = @{ AtStartup = $true }
      $randomDelay = ConvertTo-TaskTimeSpan (Get-Prop $Trigger 'random_delay')
      if ($randomDelay) { $args['RandomDelay'] = $randomDelay }
      $t = New-ScheduledTaskTrigger @args
      $delay = ConvertTo-Iso8601Duration (Get-Prop $Trigger 'delay')
      if ($delay) { $t.Delay = $delay }
      return $t
    }
    'once' {
      $args = @{ Once = $true; At = (ConvertTo-TaskDateTime (Get-Prop $Trigger 'at')) }
      $ri = ConvertTo-TaskTimeSpan (Get-Prop $Trigger 'repetition_interval')
      if ($ri) { $args['RepetitionInterval'] = $ri }
      $rd = ConvertTo-TaskTimeSpan (Get-Prop $Trigger 'repetition_duration')
      if ($rd) { $args['RepetitionDuration'] = $rd }
      $rand = ConvertTo-TaskTimeSpan (Get-Prop $Trigger 'random_delay')
      if ($rand) { $args['RandomDelay'] = $rand }
      return New-ScheduledTaskTrigger @args
    }
    'daily' {
      $args = @{ Daily = $true; At = (ConvertTo-TaskDateTime (Get-Prop $Trigger 'at')); DaysInterval = [int] (Get-Prop $Trigger 'days_interval' 1) }
      $rand = ConvertTo-TaskTimeSpan (Get-Prop $Trigger 'random_delay')
      if ($rand) { $args['RandomDelay'] = $rand }
      $t = New-ScheduledTaskTrigger @args
      $ri = ConvertTo-TaskTimeSpan (Get-Prop $Trigger 'repetition_interval')
      if ($ri) { $t.Repetition = New-TaskRepetitionPattern -Interval $ri -Duration (ConvertTo-TaskTimeSpan (Get-Prop $Trigger 'repetition_duration')) }
      return $t
    }
    'weekly' {
      $days = ConvertTo-DayOfWeekArray -Names (Get-Prop $Trigger 'days_of_week' @())
      if (@($days).Count -eq 0) { throw "scheduled_task weekly trigger requires 'days_of_week'" }
      $args = @{ Weekly = $true; At = (ConvertTo-TaskDateTime (Get-Prop $Trigger 'at')); DaysOfWeek = $days; WeeksInterval = [int] (Get-Prop $Trigger 'weeks_interval' 1) }
      $rand = ConvertTo-TaskTimeSpan (Get-Prop $Trigger 'random_delay')
      if ($rand) { $args['RandomDelay'] = $rand }
      $t = New-ScheduledTaskTrigger @args
      $ri = ConvertTo-TaskTimeSpan (Get-Prop $Trigger 'repetition_interval')
      if ($ri) { $t.Repetition = New-TaskRepetitionPattern -Interval $ri -Duration (ConvertTo-TaskTimeSpan (Get-Prop $Trigger 'repetition_duration')) }
      return $t
    }
    default { throw "Unknown scheduled_task trigger type '$type' (expected one of: logon, startup, once, daily, weekly)" }
  }
}

# --- principal ------------------------------------------------------------

function Get-AccountNamePart {
  # Task Scheduler normalizes a local account's stored UserId/GroupId down
  # to the bare account name (no 'COMPUTERNAME\'/'.\' prefix) - verified
  # empirically: declaring 'user_id: HOST\rconr' reads back as just 'rconr'.
  # Comparing the bare name (case-insensitively) avoids a false mismatch for
  # every equivalent way of spelling a local account.
  param([string] $Value)
  if (-not $Value) { return $Value }
  $idx = $Value.LastIndexOf('\')
  if ($idx -ge 0) { return $Value.Substring($idx + 1) }
  return $Value
}

function Test-TaskPrincipalMatches {
  # Only declared fields are checked - omitting 'principal' entirely means
  # "don't manage/care about it", matching 'settings' below.
  param($Item, $Task)
  $spec = Get-Prop $Item 'principal'
  if (-not $spec) { return $true }

  $userId = Get-Prop $spec 'user_id'
  $groupId = Get-Prop $spec 'group_id'
  $logonType = Get-Prop $spec 'logon_type'
  $runLevel = Get-Prop $spec 'run_level'

  if ($userId -and (Get-AccountNamePart $Task.Principal.UserId) -ne (Get-AccountNamePart $userId)) { return $false }
  if ($groupId -and (Get-AccountNamePart $Task.Principal.GroupId) -ne (Get-AccountNamePart $groupId)) { return $false }
  if ($logonType -and "$($Task.Principal.LogonType)" -ne $logonType) { return $false }
  if ($runLevel -and "$($Task.Principal.RunLevel)" -ne $runLevel) { return $false }
  return $true
}

# --- settings (partial-declare, like 'registry's 'values) -------------------

$script:SettingsFieldMap = @{
  disallow_start_if_on_batteries = @{ Property = 'DisallowStartIfOnBatteries'; Kind = 'Bool' }
  start_when_available           = @{ Property = 'StartWhenAvailable'; Kind = 'Bool' }
  hidden                          = @{ Property = 'Hidden'; Kind = 'Bool' }
  wake_to_run                     = @{ Property = 'WakeToRun'; Kind = 'Bool' }
  allow_hard_terminate            = @{ Property = 'AllowHardTerminate'; Kind = 'Bool' }
  run_only_if_network_available  = @{ Property = 'RunOnlyIfNetworkAvailable'; Kind = 'Bool' }
  run_only_if_idle                = @{ Property = 'RunOnlyIfIdle'; Kind = 'Bool' }
  multiple_instances              = @{ Property = 'MultipleInstances'; Kind = 'String' }
  restart_count                   = @{ Property = 'RestartCount'; Kind = 'Int' }
  execution_time_limit            = @{ Property = 'ExecutionTimeLimit'; Kind = 'Duration' }
  restart_interval                = @{ Property = 'RestartInterval'; Kind = 'Duration' }
  delete_expired_task_after       = @{ Property = 'DeleteExpiredTaskAfter'; Kind = 'Duration' }
}

function Test-TaskSettingsMatch {
  param($Item, $Task)
  $spec = Get-Prop $Item 'settings'
  if (-not $spec) { return $true }

  foreach ($key in $spec.Keys) {
    if (-not $script:SettingsFieldMap.Contains($key)) { Write-Warning "Unknown scheduled_task settings field '$key'; ignoring."; continue }
    $map = $script:SettingsFieldMap[$key]
    $desired = $spec[$key]
    $existing = $Task.Settings.($map.Property)
    switch ($map.Kind) {
      'Bool' { if ([bool] $desired -ne [bool] $existing) { return $false } }
      'Int' { if ([int] $desired -ne [int] $existing) { return $false } }
      'Duration' { if ((ConvertTo-TaskTimeSpan $desired) -ne (ConvertTo-TaskTimeSpan $existing)) { return $false } }
      default { if ("$desired" -ne "$existing") { return $false } }
    }
  }
  return $true
}

function Set-TaskSettingsProperties {
  param($SettingsObject, $Item)
  $spec = Get-Prop $Item 'settings'
  if (-not $spec) { return }

  foreach ($key in $spec.Keys) {
    if (-not $script:SettingsFieldMap.Contains($key)) { continue }
    $map = $script:SettingsFieldMap[$key]
    $value = $spec[$key]
    $converted =
      switch ($map.Kind) {
        'Bool' { [bool] $value }
        'Int' { [int] $value }
        'Duration' { ConvertTo-Iso8601Duration $value }
        default { $value }
      }
    $SettingsObject.($map.Property) = $converted
  }
}

# --- handler ----------------------------------------------------------------

function Get-ScheduledTaskHandler {
  [PSCustomObject]@{
    Test      = {
      param($Item)
      $name = Get-Prop $Item 'name'
      if (-not $name) { throw "scheduled_task requires a 'name'" }
      $path = Resolve-TaskFolderPath -Path (Get-Prop $Item 'path' '\')
      $existing = Get-ScheduledTask -TaskName $name -TaskPath $path -ErrorAction SilentlyContinue

      if ((Get-ItemState -Item $Item) -eq 'absent') { return [bool] $existing }
      if (-not $existing) { return $false }

      $desiredEnabled = [bool] (Get-Prop $Item 'enabled' $true)
      if ($desiredEnabled -ne ($existing.State -ne 'Disabled')) { return $false }

      $desiredDescription = Get-Prop $Item 'description'
      if ($null -ne $desiredDescription -and $existing.Description -ne $desiredDescription) { return $false }

      $desiredActions = Get-DesiredActionSpecs -Item $Item
      $existingActions = Get-ExistingActionSpecs -Task $existing
      if (-not (Test-TaskActionListsMatch -Desired $desiredActions -Existing $existingActions)) { return $false }

      $desiredTriggers = @(@(Get-Prop $Item 'triggers' @()) | ForEach-Object { ConvertTo-DesiredTriggerSpec -Trigger $_ })
      $existingTriggers = @(@($existing.Triggers) | ForEach-Object { ConvertTo-ExistingTriggerSpec -ExistingTrigger $_ })
      if (-not (Test-TaskTriggerListsMatch -Desired $desiredTriggers -Existing $existingTriggers)) { return $false }

      if (-not (Test-TaskPrincipalMatches -Item $Item -Task $existing)) { return $false }
      if (-not (Test-TaskSettingsMatch -Item $Item -Task $existing)) { return $false }

      return $true
    }
    Describe  = {
      param($Item, $Action)
      $name = Get-Prop $Item 'name'
      $path = Resolve-TaskFolderPath -Path (Get-Prop $Item 'path' '\')
      if ($Action -eq 'Uninstall') { "unregister scheduled task '$path$name'" }
      else {
        $actionCount = @(Get-Prop $Item 'actions' @()).Count
        $triggerCount = @(Get-Prop $Item 'triggers' @()).Count
        "register scheduled task '$path$name' ($actionCount action(s), $triggerCount trigger(s))"
      }
    }
    Install   = {
      param($Item)
      $name = Get-Prop $Item 'name'
      $path = Resolve-TaskFolderPath -Path (Get-Prop $Item 'path' '\')
      $description = Get-Prop $Item 'description'

      $actionSpecs = @(Get-Prop $Item 'actions' @())
      if ($actionSpecs.Count -eq 0) { Write-Warning "scheduled_task '$name' has no 'actions'; skipping."; return }

      $actions = @($actionSpecs | ForEach-Object {
        $execArgs = @{ Execute = (Get-Prop $_ 'execute') }
        $argStr = Get-Prop $_ 'arguments'
        if ($argStr) { $execArgs['Argument'] = $argStr }
        $workDir = Get-Prop $_ 'working_directory'
        if ($workDir) { $execArgs['WorkingDirectory'] = $workDir }
        New-ScheduledTaskAction @execArgs
      })

      $triggers = @(@(Get-Prop $Item 'triggers' @()) | ForEach-Object { New-TaskTriggerFromSpec -Trigger $_ })

      $registerArgs = @{ TaskName = $name; TaskPath = $path; Action = $actions; Force = $true }
      if ($triggers.Count -gt 0) { $registerArgs['Trigger'] = $triggers }
      if ($description) { $registerArgs['Description'] = $description }

      # Only build/pass a custom Settings object when the task actually
      # declares one, so a task with no 'settings' block gets Task
      # Scheduler's own untouched defaults rather than whatever
      # New-ScheduledTaskSettingsSet's own baseline happens to be.
      $settingsSpec = Get-Prop $Item 'settings'
      if ($settingsSpec) {
        $settingsObject = New-ScheduledTaskSettingsSet
        Set-TaskSettingsProperties -SettingsObject $settingsObject -Item $Item
        $registerArgs['Settings'] = $settingsObject
      }

      $principalSpec = Get-Prop $Item 'principal'
      if ($principalSpec) {
        $logonType = Get-Prop $principalSpec 'logon_type'
        $runLevel = Get-Prop $principalSpec 'run_level'

        if ($logonType -eq 'Password') {
          $userId = Get-Prop $principalSpec 'user_id'
          if (-not $userId) { throw "scheduled_task '$name' principal.logon_type is 'Password' but no 'user_id' was given" }
          $passwordEnvName = Get-Prop $principalSpec 'password_env'
          $password = if ($passwordEnvName) { [Environment]::GetEnvironmentVariable($passwordEnvName) } else { $null }
          if (-not $password) { throw "scheduled_task '$name' principal.logon_type is 'Password' but 'password_env' ($passwordEnvName) is not set" }
          $registerArgs['User'] = $userId
          $registerArgs['Password'] = $password
          if ($runLevel) { $registerArgs['RunLevel'] = $runLevel }
        } else {
          $principalArgs = @{}
          $groupId = Get-Prop $principalSpec 'group_id'
          $userId = Get-Prop $principalSpec 'user_id'
          if ($groupId) { $principalArgs['GroupId'] = $groupId }
          elseif ($userId) { $principalArgs['UserId'] = $userId }
          if ($logonType) { $principalArgs['LogonType'] = $logonType }
          if ($runLevel) { $principalArgs['RunLevel'] = $runLevel }
          $registerArgs['Principal'] = New-ScheduledTaskPrincipal @principalArgs
        }
      }

      Register-ScheduledTask @registerArgs | Out-Null

      if ([bool] (Get-Prop $Item 'enabled' $true)) { Enable-ScheduledTask -TaskName $name -TaskPath $path | Out-Null }
      else { Disable-ScheduledTask -TaskName $name -TaskPath $path | Out-Null }
    }
    Uninstall = {
      param($Item)
      $name = Get-Prop $Item 'name'
      $path = Resolve-TaskFolderPath -Path (Get-Prop $Item 'path' '\')
      Unregister-ScheduledTask -TaskName $name -TaskPath $path -Confirm:$false -ErrorAction SilentlyContinue
    }
  }
}

Export-ModuleMember -Function Get-ScheduledTaskHandler
