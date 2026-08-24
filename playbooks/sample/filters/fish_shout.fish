#!/usr/bin/env fish

# this filter uppercases the value and adds an exclamation point. It is a silly example, but it shows how to write a filter in fish.
if test -z "$argv[1]"
	echo ""
	exit 0
end
echo -n (string upper $argv[1])
echo -n '!'
