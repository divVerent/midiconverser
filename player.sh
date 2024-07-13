#!/bin/sh

player=$1
verses=$2
prefix=$3

panic() {
	$player "$prefix.panic.mid"
	exit 1
}

waitkey() {
	stty -icanon -echo
	head -c 1 >/dev/null
	stty icanon echo
}

trap panic INT

if [ -f "$prefix.prelude.mid" ]; then
	echo "Playing prelude..."
	$player "$prefix.prelude.mid"
	needwait=true
fi

for i in $(seq 1 "$verses"); do
	state=init
	for part in $prefix.part*.mid; do
		case "$state" in
			init)
				text="START VERSE $i?"
				answer="Playing verse $i..."
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
		$player "$part"
		needwait=true
	done
done
echo "Done."
