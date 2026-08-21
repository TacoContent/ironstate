param($Value, [object[]]$ArgValues)
# null-in/null-out, matching every other filter here (trim/upper/quote/...) -
# a field like 'marker_name: "${{ item.public_key | sha1 }}"' gets resolved
# unconditionally for every loop iteration regardless of that branch's own
# 'when' (sibling fields aren't 'when'-gated - see lookup.ps1's own comment),
# so an iteration where 'item.public_key' is unset must flow through as
# null, not throw.
if ($null -eq $Value) { return $null }
if ($Value -isnot [string]) { throw "sha1 filter requires a string value" }
$sha1 = [System.Security.Cryptography.SHA1]::Create()
$bytes = [System.Text.Encoding]::UTF8.GetBytes($Value)
$hashBytes = $sha1.ComputeHash($bytes)
$hashString = [BitConverter]::ToString($hashBytes) -replace '-', ''
return $hashString.ToLower()