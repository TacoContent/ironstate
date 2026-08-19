param($Value, [object[]] $ArgValues)
if ([string]::IsNullOrWhiteSpace($Value)) { return $null }
return $Value
