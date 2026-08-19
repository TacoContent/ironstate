param($Value, [object[]] $ArgValues)
if ($null -eq $Value) { return $null }
if ($ArgValues.Count -lt 1) { throw "'concat' filter expects at least 1 argument" }
return -join (@([string] $Value) + @($ArgValues | ForEach-Object { [string] $_ }))