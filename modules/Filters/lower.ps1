param($Value, [object[]] $ArgValues)
if ($null -eq $Value) { return $null }
return ([string] $Value).ToLowerInvariant()
