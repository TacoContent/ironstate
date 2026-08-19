param($Value, [object[]] $ArgValues)
if ($null -eq $Value) { return $null }
if ([string]::IsNullOrWhiteSpace($Value)) { return $null }
$q = '"'
if ($ArgValues.Count -eq 1) { $q = $ArgValues[0] }
return "$q$Value$q"
