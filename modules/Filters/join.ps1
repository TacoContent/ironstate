param($Value, [object[]] $ArgValues)
if ($null -eq $Value) { return $null }
if ($ArgValues.Count -lt 1) { throw "'join' filter expects at least 1 argument" }
$parts = @([string] $Value) + @($ArgValues | ForEach-Object { [string] $_ })
return [System.IO.Path]::Combine([string[]] $parts)