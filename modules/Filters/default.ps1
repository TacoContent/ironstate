# Every filter is null-in/null-out except this one, which is the one that
# *handles* null - that's what lets a package write
# '${{ languages.java.jdk | default("Oracle.JDK.25") }}' to mean "use the
# user's override if they set one, else this built-in default".
param($Value, [object[]] $ArgValues)
if ($ArgValues.Count -ne 1) { throw "'default' filter expects exactly 1 argument" }
if ($null -eq $Value) { return $ArgValues[0] }
return $Value
