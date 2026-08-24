#!/usr/bin/env zsh
# Persistent JSON-over-stdio filter shim (docs/plans/go-rewrite.md §4.5) for
# zsh script filters - the '.zsh' counterpart to shim.sh.
#
# Usage: shim.zsh <path-to-filter-script.zsh>
#
# Protocol: identical to shim.sh - one line of
# '{"value":<json>,"args":[<json>...]}' read from stdin per call; one line
# of '{"result":<json>}' or '{"error":"..."}' written to stdout in
# response. Kept alive for the process's lifetime.
#
# Requires 'jq' on PATH (zsh has no native JSON support) - a discovered
# .zsh filter fails per-call (not a hard crash) if jq is missing.
#
# The target script itself only ever sees plain positional args - $1 is
# the value (empty string for a null value), $2..$n are the call's args,
# each coerced to a string - and its stdout, with trailing newlines
# trimmed, becomes the string result. A zsh filter can only ever return a
# string.
script_path="$1"

while IFS= read -r line; do
	[ -z "$line" ] && continue

	if ! command -v jq >/dev/null 2>&1; then
		print -r -- '{"error":"zsh script filters require jq on PATH to decode/encode requests"}'
		continue
	fi

	if ! decoded=$(jq -j '
		(if .value == null then "" else (.value | tostring) end) + "\u0000" +
		((.args // []) | map(if . == null then "" else tostring end) | join("\u0000"))
	' <<<"$line" 2>/dev/null); then
		print -r -- '{"error":"invalid request JSON"}'
		continue
	fi

	fields=("${(@ps:\0:)decoded}")
	value="${fields[1]}"
	args=("${fields[@]:1}")

	errfile=$(mktemp)
	if output=$(zsh "$script_path" "$value" "${args[@]}" 2>"$errfile"); then
		jq -n -c --arg r "$output" '{result: $r}'
	else
		errtext=$(cat "$errfile" 2>/dev/null)
		jq -n -c --arg e "${errtext:-script exited with a non-zero status}" '{error: $e}'
	fi
	rm -f "$errfile"
done
