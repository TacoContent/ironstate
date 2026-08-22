param($Value, [object[]] $ArgValues)
# The json_query filter takes a single input, which is a PowerShell object resulting from parsing JSON data. if jq is installed, it uses jq to query the JSON data using the provided jq filter. If jq is not installed, it uses the built-in ConvertTo-Json and ConvertFrom-Json cmdlets to query the JSON data.
if ($ArgValues.Length -eq 0) { throw "json_query filter requires at least one argument" }
$jq_filter = $ArgValues[0].ToString()
if (Get-Command jq -ErrorAction SilentlyContinue) {
    # Use jq to query the JSON data
    $json_input = $Value | ConvertTo-Json -Compress -Depth 100
    $result = $json_input | & jq -r $jq_filter
    return $result | ConvertFrom-Json
} else {
    # Use built-in PowerShell cmdlets to query the JSON data
    $json_object = $Value | ConvertTo-Json -Compress -Depth 100 | ConvertFrom-Json
    return $json_object | Select-Object -ExpandProperty $jq_filter
}