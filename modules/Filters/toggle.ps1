# Like 'default', but treats a bare boolean the same as unset - for a var
# that's normally an on/off flag (e.g. 'jdk: true' meaning "install it, with
# the built-in default package") but may instead be set to a string to name
# a specific override (e.g. 'jdk: Eclipse.Temurin.21'). Only a string counts
# as an explicit override; null *or* a boolean (true or false) falls back -
# a 'when:' clause is the right place to act on the true/false distinction
# itself (see packages/languages/java).
param($Value, [object[]] $ArgValues)
if ($ArgValues.Count -ne 1) { throw "'toggle' filter expects exactly 1 argument" }
if ($Value -is [string]) { return $Value }
return $ArgValues[0]
