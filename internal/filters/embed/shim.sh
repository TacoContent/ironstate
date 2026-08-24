#!/usr/bin/env bash
# Persistent JSON-over-stdio filter shim (docs/plans/go-rewrite.md §4.5) for
# bash script filters - the '.sh' counterpart to shim.ps1.
#
# Usage: shim.sh <path-to-filter-script.sh>
#
# Protocol: identical to shim.ps1 - one line of
# '{"value":<json>,"args":[<json>...]}' read from stdin per call; one line
# of '{"result":<json>}' or '{"error":"..."}' written to stdout in
# response. Kept alive for the process's lifetime.
#
# Unlike PowerShell (ConvertFrom/To-Json built in), bash has no native JSON
# support, so this shim requires 'jq' on PATH to decode/encode the
# protocol - a discovered .sh filter fails per-call (not a hard crash) if
# jq is missing. Requires bash >= 4 (readarray/mapfile).
#
# The target script itself only ever sees plain positional args - $1 is
# the value (empty string for a null value), $2..$n are the call's args,
# each coerced to a string (a non-string value/arg is passed as its
# compact JSON text) - and its stdout, with trailing newlines trimmed,
# becomes the string result. A bash filter can only ever return a string.
set -uo pipefail

script_path="$1"

while IFS= read -r line; do
	[ -z "$line" ] && continue

	if ! command -v jq >/dev/null 2>&1; then
		printf '%s\n' '{"error":"bash script filters require jq on PATH to decode/encode requests"}'
		continue
	fi

	if ! decoded=$(jq -j '
		(if .value == null then "" else (.value | tostring) end) + "\u0000" +
		((.args // []) | map(if . == null then "" else tostring end) | join("\u0000"))
	' <<<"$line" 2>/dev/null); then
		printf '%s\n' '{"error":"invalid request JSON"}'
		continue
	fi

	readarray -d '' -t fields <<<"$decoded"
	value="${fields[0]:-}"
	args=("${fields[@]:1}")

	errfile=$(mktemp)
	if output=$(bash "$script_path" "$value" "${args[@]}" 2>"$errfile"); then
		jq -n -c --arg r "$output" '{result: $r}'
	else
		errtext=$(cat "$errfile" 2>/dev/null)
		jq -n -c --arg e "${errtext:-script exited with a non-zero status}" '{error: $e}'
	fi
	rm -f "$errfile"
done
