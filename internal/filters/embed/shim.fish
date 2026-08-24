#!/usr/bin/env fish
# Persistent JSON-over-stdio filter shim (docs/plans/go-rewrite.md §4.5) for
# fish script filters - the '.fish' counterpart to shim.sh/shim.zsh.
#
# Usage: shim.fish <path-to-filter-script.fish>
#
# Protocol: identical to shim.sh/shim.zsh - one line of
# '{"value":<json>,"args":[<json>...]}' read from stdin per call; one line
# of '{"result":<json>}' or '{"error":"..."}' written to stdout in
# response. Kept alive for the process's lifetime.
#
# Requires 'jq' on PATH (fish has no native JSON support) - a discovered
# .fish filter fails per-call (not a hard crash) if jq is missing. The
# target script is run with --no-config (unlike bash/zsh, fish loads
# config.fish/conf.d/* even for non-interactive script runs by default -
# a filter's behavior shouldn't depend on the caller's own fish config).
#
# The target script sees $argv[1] as the value (empty string for a null
# value), $argv[2..] as the call's args (each coerced to a string), and
# its stdout, with trailing newlines trimmed, becomes the string result.
# A fish filter can only ever return a string.
set script_path $argv[1]

while read -l line
	if test -z "$line"
		continue
	end

	if not command -v jq >/dev/null 2>&1
		echo '{"error":"fish script filters require jq on PATH to decode/encode requests"}'
		continue
	end

	if not set decoded (printf '%s' "$line" | jq -j '
		(if .value == null then "" else (.value | tostring) end) + "\u0000" +
		((.args // []) | map(if . == null then "" else tostring end) | join("\u0000"))
	' 2>/dev/null)
		echo '{"error":"invalid request JSON"}'
		continue
	end

	set fields (printf '%s' "$decoded" | string split0)
	set value $fields[1]
	set args $fields[2..-1]

	set errfile (mktemp)
	set output (fish --no-config $script_path $value $args 2>$errfile)
	if test $status -eq 0
		set joined (string join \n -- $output)
		jq -n -c --arg r "$joined" '{result: $r}'
	else
		set errtext (cat $errfile 2>/dev/null)
		if test -z "$errtext"
			set errtext "script exited with a non-zero status"
		end
		jq -n -c --arg e "$errtext" '{error: $e}'
	end
	rm -f $errfile
end
