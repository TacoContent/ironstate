# Collapses the repeated "(X is mapping and X.Y) or (X is boolean and X)"
# on/off-toggle pattern (see e.g. packages/languages/main.yml) into one
# filter call, generalized to any depth: 'languages | enabled("rust")' -
# or, for a deeper path, 'productivity | enabled("browsers", "chrome")'.
#
# A filter only ever sees the value piped into it, never the surrounding
# variable context - so this can't be spelled as
# 'productivity.browsers.chrome | enabled()' with no arguments: by the time
# that whole dotted path resolves to one value, an ancestor level that was a
# boolean has already collapsed everything below it to null (a boolean isn't
# a mapping, so the path walk stops there), silently losing whether that
# ancestor was 'true' or 'false'. Piping the *top* value and passing the
# remaining path as plain string arguments keeps every level visible to this
# filter's own walk.
#
# Walks $Value through $ArgValues as successive mapping keys. At each step,
# a boolean value stops the walk immediately and *is* the answer, at
# whatever level it was found - 'true' enables everything below it, 'false'
# disables everything below it regardless of what a deeper key says (an
# explicit disable always wins). Once every argument is consumed (or none
# were given - a bare 'X | enabled' checks X itself), the value reached is
# "on" if it's boolean 'true' *or* still a mapping - present/configured in
# some structured way, even without a specific leaf checked yet (e.g. a
# wrapping task deciding whether to even look at a 'browsers:' section at
# all, before any one browser's own key is checked). Anything else (a
# missing key, a non-mapping in the way, or a non-mapping/non-boolean like a
# string or number at the end) is "off".
param($Value, [object[]] $ArgValues)

$current = $Value
foreach ($key in $ArgValues) {
  if ($current -is [bool]) { return $current }
  if ($current -isnot [System.Collections.IDictionary] -or -not $current.Contains([string] $key)) { return $false }
  $current = $current[[string] $key]
}

if ($current -is [bool]) { return $current }
if ($current -is [System.Collections.IDictionary]) { return $true }
return $false
