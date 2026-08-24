#!/usr/bin/env zsh

# this filter uppercases the value and adds an exclamation point. It is a silly example, but it shows how to write a filter in zsh.
if [ -z "$1" ]; then
	echo ""
	exit 0
fi
echo -n "$1" | tr '[:lower:]' '[:upper:]'
echo -n '!'
