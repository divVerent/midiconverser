#!/bin/sh

player=$1
verses=$2
prefix=$3

panic() {
	$player "$prefix.panic.mid"
	exit 1
}

trap panic INT

if [ -f "$prefix.prelude.mid" ]; then
	echo "Playing prelude..."
	$player "$prefix.prelude.mid"
	needwait=true
fi

for i in $(seq 1 "$verses"); do
	echo "Playing verse $i..."
	state=init
	for part in $prefix.part*.mid; do
		case "$state" in
			init)
				text="START VERSE 1?"
				state=hold
				;;
			hold)
				text="HOLD FERMATA?"
				state=cont
				;;
			cont)
				text="CONTINUE?"
				state=hold
				;;
		esac
		if $needwait; then
			figlet "$text"
			read
		fi
		$player "$part"
		needwait=true
	done
done
echo "Done."
