#!/usr/bin/env bash

# create a filter, like one of the powershell ones
# this is the powershell style:
#
# param($Value, [object[]] $ArgValues)
# if ($null -eq $Value) { return $null }
# # replace all lowercase consonants with uppercase consonants, leaving vowels and non-letters alone
# return $Value -replace '([bcdfghjklmnpqrstvwxyz])', { $_.Value.ToUpper() }

# this is the bash style:

# $1 is the value to filter
# $2..$n are the arguments to the filter

# Force the C locale: under some locales, bash's [A-Z]/[0-9] bracket
# ranges are collation-order-dependent and can match unexpected
# characters (e.g. lowercase letters) - see POSIX's locale-dependent
# range expressions. Pin to C so [A-Z]/[0-9] mean exactly what they say.
export LC_ALL=C

# this filter replaces every word with "buffalo" (or "Buffalo" if the word is capitalized) and leaves punctuation alone. It is a silly example, but it shows how to write a filter in bash.
if [ -z "$1" ]; then
	echo ""
	exit 0
fi

for word in $1; do
	if [[ $word =~ ^[A-Z] ]]; then
		echo -n "Buffalo "
	# if the word is numbers, leave it alone
	elif [[ $word =~ ^[0-9] ]]; then
		echo -n "$word "
	else
		echo -n "buffalo "
	fi
done
