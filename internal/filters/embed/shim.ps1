# Persistent JSON-over-stdio filter shim (docs/plans/go-rewrite.md §4.5).
#
# Usage: shim.ps1 <path-to-filter-script.ps1>
#
# Protocol: one line of '{"value":<json>,"args":[<json>...]}' read from
# stdin per call; one line of '{"result":<json>}' or '{"error":"..."}'
# written to stdout in response. Kept alive for the process's lifetime
# (internal/filters' Go side keeps this process warm rather than spawning
# one per call) so an existing modules/Filters/*.ps1 file's own
# 'param($Value, [object[]] $ArgValues)' contract needs zero changes.
param([Parameter(Mandatory)][string] $ScriptPath)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

while ($true) {
  $line = [Console]::In.ReadLine()
  if ($null -eq $line) { break }
  if ([string]::IsNullOrWhiteSpace($line)) { continue }

  try {
    $request = $line | ConvertFrom-Json
    $value = $request.value
    $argValues = @($request.args)
    $result = & $ScriptPath -Value $value -ArgValues $argValues
    $response = @{ result = $result }
  } catch {
    $response = @{ error = $_.Exception.Message }
  }

  $json = ConvertTo-Json -InputObject $response -Compress -Depth 10
  [Console]::Out.WriteLine($json)
  [Console]::Out.Flush()
}
