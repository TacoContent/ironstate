param($Value, [object[]] $ArgValues)
# The from_json filter takes a single argument, which is a string containing JSON data. It parses the JSON and returns the corresponding PowerShell object.
return $Value | ConvertFrom-Json