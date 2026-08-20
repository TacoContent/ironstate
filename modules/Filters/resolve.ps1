param($Value, [object[]] $ArgValues)
if ($null -eq $Value) { return $null }
# if argvalues are provided, it should error, because no args are expected
if ($ArgValues -and $ArgValues.Count -gt 0) {
    throw "Resolve-UserPath does not accept argument values."
}
if (Get-Command Resolve-UserPath) {
    return Resolve-UserPath -Path $Value
}

return $Value
