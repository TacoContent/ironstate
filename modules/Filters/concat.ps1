param($Value, [object[]] $ArgValues)
if ($null -eq $Value) { return $null }
if ($ArgValues.Count -lt 1) { throw "'concat' filter expects at least 1 argument" }
$delimiter = [string] $ArgValues[0]
# pop the first argument off the list of arguments, which is the delimiter; the rest, if any, are extra items to append
$extraItems = if ($ArgValues.Count -gt 1) { $ArgValues[1..($ArgValues.Count - 1)] } else { @() }

# $Value may be a single string/scalar or an array of items (e.g. from json_query) - either way, flatten to a list of strings before joining
$items = [System.Collections.Generic.List[string]]::new()
if ($Value -is [string]) {
    $items.Add($Value)
} else {
    foreach ($item in @($Value)) { $items.Add([string] $item) }
}
foreach ($item in $extraItems) { $items.Add([string] $item) }

return ($items -join $delimiter)