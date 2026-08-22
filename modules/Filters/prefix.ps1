# Prepends a fixed string + single space to a value, or to every element of
# an array value (e.g. building "hostname key" lines from a list of known
# hosts keys: 'item.key | split("\n") | prefix(item.hostnames | concat(" "))').
# Null-in/null-out.
param($Value, [object[]] $ArgValues)
if ($null -eq $Value) { return $null }
if ($ArgValues.Count -ne 1) { throw "'prefix' filter expects exactly 1 argument" }
$prefix = [string] $ArgValues[0]
if ($Value -is [string]) { return "$prefix $Value" }
$results = [System.Collections.Generic.List[string]]::new()
foreach ($item in @($Value)) { $results.Add("$prefix $item") }
# ',' - see Filters/default.ps1's note: without it, a 0- or 1-element result
# unrolls into $null / a bare string instead of surviving as an array.
return ,$results.ToArray()
