param($Value, [object[]] $ArgValues)
# if $ArgValues contains a single boolean, we expect that to be the return comparison value, otherwise we default to true
$expected = $true
$ArgValues | ForEach-Object {
    if ($_ -is [bool]) {
        $expected = $_
    }
}
if ($null -eq $Value) { return -not $expected }
# if $Value is null or whitespace, return false
if ($Value -is [string] -and [string]::IsNullOrWhiteSpace($Value)) { return -not $expected }

if ($Value -is [string]) { return ($expected -eq (Test-Path $Value)) }
if ($Value -is [System.IO.FileInfo]) { return ($expected -eq $Value.Exists) }
if ($Value -is [System.IO.DirectoryInfo]) { return ($expected -eq $Value.Exists) }

return -not $expected