# Every filter is null-in/null-out except this one, which is the one that
# *handles* null - that's what lets a package write
# '${{ languages.java.jdk | default("Oracle.JDK.25") }}' to mean "use the
# user's override if they set one, else this built-in default".
param($Value, [object[]] $ArgValues)
if ($ArgValues.Count -ne 1) { throw "'default' filter expects exactly 1 argument" }
if ($null -eq $Value) {
  # ',' only when the default itself is array-shaped (e.g.
  # 'default([])' for an optional list field): otherwise this return
  # statement's own implicit enumeration would unroll a 0- or 1-element
  # default array into $null / a bare element instead of the array value
  # itself - same hazard noted in Expressions.psm1's 'List'/'Filter' cases.
  # A scalar default is returned as-is - comma-wrapping it here would
  # incorrectly turn it into a 1-element array for every existing caller.
  $default = $ArgValues[0]
  if (($default -is [System.Collections.IEnumerable]) -and ($default -isnot [string])) { return ,$default }
  return $default
}
return $Value
