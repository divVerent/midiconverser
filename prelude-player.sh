#!/bin/sh

player=$1
repeat=${2:-2}
sleep=${3:-2}

prefix=

panic() {
	echo "Stopping all notes..."
	if [ -n "$prefix" ]; then
		$player "$prefix.panic.mid"
	fi
	exit 1
}

trap panic EXIT

trap 'exit 1' INT

LF='
'

while :; do
	verses=$(echo *.verse.mid | xargs -n 1 | shuf)"$LF"
	while [ -n "$verses" ]; do
		verse=${verses%%$LF*}
		verses=${verses#*$LF}
		prefix=${verse%.verse.mid}
		# No xmas songs except in December.
		case "$prefix" in
			12??)
				case "$(date +%Y%m%d)" in
					????12[01]?|????122[0-6])
						;;
					*)
						echo "Skipping $prefix due to it not being Christmas."
						continue
						;;
				esac
				;;
		esac
		thisrepeat=$repeat
		# HACK; better migrate to a JSON file.
		if grep -q -- -verses=1 "$prefix.sh"; then
			# If -verses=1, then repeats are baked in.
			# These are too long - skip them.
			echo "Skipping $prefix due to baked-in repeats."
			continue
		fi
		for i in $(seq 1 $thisrepeat); do
			echo "Playing repeat $i/$thisrepeat of verse of $prefix..."
			$player "$prefix.verse.mid"
			echo "Waiting $sleep seconds..."
			sleep 2
		done
	done
done
