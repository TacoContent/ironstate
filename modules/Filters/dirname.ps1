param($Value, [object[]] $ArgValues)
if ($null -eq $Value) { return $null }
return [System.IO.Path]::GetDirectoryName([string] $Value)