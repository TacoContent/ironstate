#!/usr/bin/env bash

# this filter takes every constant letter and replaces it with leet speak. It is a silly example, but it shows how to write a filter in bash.
if [ -z "$1" ]; then
	echo ""
	exit 0
fi

# letters to replace with leet speak
declare -A leet_map=(
	[a]='4'
	[e]='3'
	[f]=ph
	[g]='9'
	[i]='1'
	[o]='0'
	[s]='5'
	[t]='7'
	[z]='2'
)

for word in $1; do
	leet_word=""
	for (( i=0; i<${#word}; i++ )); do
		char="${word:$i:1}"
		lower_char=$(echo "$char" | tr '[:upper:]' '[:lower:]')
		if [[ -n "${leet_map[$lower_char]}" ]]; then
			leet_word+="${leet_map[$lower_char]}"
		else
			leet_word+="$char"
		fi
	done
	echo -n "$leet_word "
done
