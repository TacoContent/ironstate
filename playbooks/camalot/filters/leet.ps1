param($Value, [object[]] $ArgValues)
if ($null -eq $Value) { return $null }
# replace all lowercase consonants with uppercase consonants, leaving vowels and non-letters alone
return $Value -replace '([bcdfghjklmnpqrstvwxyz])', { $_.Value.ToUpper() }