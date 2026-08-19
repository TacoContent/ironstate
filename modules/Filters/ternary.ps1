param($Value, [object[]] $ArgValues)
if ($ArgValues.Count -ne 2) { throw "'ternary' filter expects exactly 2 arguments" }
if ($Value) { return $ArgValues[0] } else { return $ArgValues[1] }
