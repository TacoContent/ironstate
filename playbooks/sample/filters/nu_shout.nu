# this filter uppercases the value and adds an exclamation point. It is a silly example, but it shows how to write a filter in nushell.
def main [value, ...args] {
  if ($value | is-empty) {
    print ""
  } else {
    print $"($value | str upcase)!"
  }
}
