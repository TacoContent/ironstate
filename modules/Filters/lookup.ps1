param($Value, [object[]] $ArgValues)
if ($ArgValues.Length -eq 0) { throw "lookup filter requires at least one argument" }

$action = $ArgValues[0].ToString().ToLower()

# Every action after the first argument is one or more pieces to
# concatenate into the actual URL/path - e.g.
# 'lookup("url", "https://github.com/", item.github_user, ".keys")'. If ANY
# piece is null/empty (e.g. an unset 'item.github_user' for a loop entry
# that's really a 'public_key'/'file' entry instead), the whole lookup is
# skipped (returns $null, same "null-in/null-out" convention every other
# filter here follows) rather than silently composing a subtly-wrong target
# - like 'https://github.com/.keys' - and actually firing a request/read
# against it.
# Built with a plain loop/.Add(), not a '|' pipeline ('Select-Object -Skip
# 1' etc.) - piping a single $null element through the pipeline silently
# drops it instead of yielding one $null-valued object, which would make a
# lone null piece (the exact case this whole check exists to catch)
# disappear as if no piece had been given at all - same enumeration hazard
# called out throughout Expressions.psm1/Tasks.psm1.
$pieces = [System.Collections.Generic.List[object]]::new()
for ($i = 1; $i -lt $ArgValues.Length; $i++) { $pieces.Add($ArgValues[$i]) }
if ($pieces.Count -eq 0) { throw "lookup filter '$action' action requires at least one more argument" }
foreach ($piece in $pieces) {
  if ($null -eq $piece -or ($piece -is [string] -and [string]::IsNullOrEmpty($piece))) { return $null }
}
$target = -join ($pieces | ForEach-Object { [string] $_ })

switch ($action) {
  "url" {
    return (Invoke-WebRequest -Uri $target -UseBasicParsing).Content
  }
  "file" {
    $path = $target
    if (Get-Command Resolve-UserPath -ErrorAction SilentlyContinue) { $path = Resolve-UserPath -Path $path }
    if (-not (Test-Path -Path $path -PathType Leaf)) { return $null }
    return (Get-Content -Path $path -Raw)
  }
  default {
    throw "lookup filter does not support action '$action'"
  }
}
