#!/usr/bin/env -S nu --stdin --no-config-file
# Single-shot JSON filter shim (docs/plans/go-rewrite.md §4.5) for
# nushell script filters - the '.nu' counterpart to shim.ps1/shim.sh/
# shim.zsh/shim.fish.
#
# Unlike those shells, nu's $in/stream primitives always collect (wait
# for stdin to fully close) before yielding a value - there is no
# reliable way to read one request line, respond, then read a later
# request line on the same still-open stdin pipe. So '.nu' filters are
# NOT run through a persistent per-filter worker process kept warm across
# calls (see pool.go) - script.go's oneshotFilterFunc spawns a fresh
# 'nu --stdin --no-config-file shim.nu <script_path>' process per call
# instead, writes one line of request to its stdin, closes stdin, and
# reads its one line of response before the process exits.
#
# Usage: nu --stdin --no-config-file shim.nu <path-to-filter-script.nu>
#
# Protocol: same shape as every other shim - a single line of
# '{"value":<json>,"args":[<json>...]}' read from stdin; a single line of
# '{"result":<json>}' or '{"error":"..."}' written to stdout.
#
# The target script must define 'def main [value, ...args] { ... }' (nu's
# own idiomatic script-argument convention) and print its result via
# 'print' - it is run as its own separate 'nu' process (script.go builds
# no special contract beyond ordinary CLI args, all coerced to strings,
# same as bash/zsh/fish), so like those shells a nu filter's result is
# always a plain string, not an arbitrary nu/JSON value.
def main [script_path: string] {
    let request = ($in | from json)
    let value = ($request | get -o value)
    let args = ($request | get -o args | default [])
    let call_args = ([$value] | append $args)

    let outcome = (^nu --no-config-file $script_path ...$call_args | complete)
    if $outcome.exit_code == 0 {
        let text = ($outcome.stdout | str trim -r)
        print ({result: $text} | to json -r)
    } else {
        let errtext = ($outcome.stderr | str trim)
        let final_err = (if ($errtext | is-empty) { "script exited with a non-zero status" } else { $errtext })
        print ({error: $final_err} | to json -r)
    }
}
