#!/bin/sh

prefix=$1
verses=$2

LF='
'

if [ -z "$ALSA_PORT" ]; then
	ALSA_PORT=$(
		aplaymidi -l | tail -n +2 | while read -r port rest; do
			prio=
			case "$rest" in
				Midi\ Through\ *)
					# Internal port - ignore.
					continue
					;;
				FLUID\ *)
					# Known soft synth - low prio.
					prio=1
					;;
				*USB*|UM-*)
					# Known external port - high prio.
					prio=3
					;;
				*)
					# Likely external device.
					prio=2
					;;
			esac
			echo "$prio $port $rest"
		done | sort -n | tail -n 1 | cut -d ' ' -f 2-
	)
	if [ -z "$ALSA_PORT" ]; then
		echo >&2 "No ALSA port detected; please export ALSA_PORT=yourport to force."
		exit 1
	fi
	desc=${ALSA_PORT#* }
	ALSA_PORT=${ALSA_PORT%% *}
	echo >&2 "Autodetected ALSA port: $ALSA_PORT ($desc); export ALSA_PORT=yourport to override."
fi

player() {
	aplaymidi -p "$ALSA_PORT" -d 0 "$@"
}

panic() {
	echo "Stopping all notes..."
	if [ -n "$prefix" ]; then
		player "$prefix.panic.mid"
	fi
	exit 1
}

trap panic EXIT

trap 'exit 1' INT

waitkey() {
	stty -icanon -echo
	head -c 1 >/dev/null
	stty icanon echo
}

if [ -n "$prefix" ]; then
	# Hymn player.
	if [ -z "$verses" ]; then
		verses=$(jq < "$prefix.json" .num_verses)
	fi
	echo "Note: will play $verses verses."

	if [ -f "$prefix.prelude.mid" ]; then
		echo "Playing prelude..."
		player "$prefix.prelude.mid"
		needwait=true
	fi

	for i in $(seq 1 "$verses"); do
		state=init
		for part in $prefix.part*.mid; do
			case "$state" in
				init)
					text="START VERSE $i?"
					answer="Playing verse $i/$verses..."
					state=hold
					;;
				hold)
					text="END FERMATA?"
					answer="Silence..."
					state=cont
					;;
				cont)
					text="CONTINUE?"
					answer="Continuing..."
					state=hold
					;;
			esac
			if $needwait; then
				figlet "$text"
				waitkey
			fi
			echo "$answer"
			player "$part"
			needwait=true
		done
	done
	echo "Done."
else
	# Prelude player.
	: ${repeat:=2}
	: ${sleep:=2}
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
			if [ x"$(jq < "$prefix.json" .num_verses)" = x"1" ]; then
				# If -verses=1, then repeats are baked in.
				# These are too long - skip them.
				echo "Skipping $prefix due to baked-in repeats."
				continue
			fi
			for i in $(seq 1 $thisrepeat); do
				echo "Playing repeat $i/$thisrepeat of verse of $prefix..."
				player "$prefix.verse.mid"
				echo "Waiting $sleep seconds..."
				sleep 2
			done
		done
	done
fi
