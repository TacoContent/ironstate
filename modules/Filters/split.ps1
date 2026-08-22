# Splits a string into an array on a literal delimiter (the inverse of the
# 'concat'/'join' filters). Null-in/null-out. A trailing empty element (from
# a trailing delimiter) is dropped, matching how 'concat' never adds one -
# splitting a value 'concat' produced round-trips cleanly.
param($Value, [object[]] $ArgValues)
if ($null -eq $Value) { return $null }
if ($ArgValues.Count -ne 1) { throw "'split' filter expects exactly 1 argument" }
$delimiter = [string] $ArgValues[0]
$parts = [System.Collections.Generic.List[string]]::new()
foreach ($part in ([string] $Value).Split($delimiter)) { $parts.Add($part) }
if ($parts.Count -gt 0 -and $parts[$parts.Count - 1] -eq '') { $parts.RemoveAt($parts.Count - 1) }
# ',' - see Filters/default.ps1's note: without it, a 0- or 1-element result
# unrolls into $null / a bare string instead of surviving as an array.
return ,$parts.ToArray()
