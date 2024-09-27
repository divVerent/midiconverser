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
				FLUID\ *|TiMidity\ *)
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
	stty icanon echo
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
		verses=$(yq < "$prefix.yml" .num_verses)
	fi
	echo "Note: will play $verses verses."

	if [ -f "$prefix.prelude.mid" ]; then
		echo "Playing prelude..."
		player "$prefix.prelude.mid"
		needwait=true
	fi

	for i in $(seq 1 "$verses"); do
		state=init
		part=0
		while [ -f "$prefix.part$part.mid" ]; do
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
			player "$prefix.part$part.mid"
			needwait=true
			part=$((part + 1))
		done
	done

	if [ -f "$prefix.postlude.mid" ]; then
		if $needwait; then
			figlet "START POSTLUDE?"
			waitkey
		fi
		echo "Playing postlude..."
		player "$prefix.postlude.mid"
	fi

	echo "Done."
else
	repeat=$(yq < config.yml '.prelude_player_repeat // 2')
	sleep=$(yq < config.yml '.prelude_player_sleep_sec // 2')
	while :; do
		verses=$(echo *.verse.mid | xargs -n 1 | shuf)"$LF"
		while [ -n "$verses" ]; do
			verse=${verses%%$LF*}
			verses=${verses#*$LF}
			prefix=${verse%.verse.mid}
			# No xmas songs except in December.
			# TODO move this into the yml files.
			case "$prefix" in
				20[1-9]|21[0-4]|12??)
					case "$(date +%Y%m%d)" in
						????12[01]?|????122[0-6])
							;;
						*)
							echo "Skipping $prefix due to it not being Christmas."
							continue
							;;
					esac
					;;
				33[89]|34[01])
					echo "Skipping $prefix due to it being for special occasions."
					continue
					;;
			esac
			thisrepeat=$repeat
			# HACK; better migrate to a JSON file.
			if [ x"$(yq < "$prefix.yml" .num_verses)" = x"1" ]; then
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
