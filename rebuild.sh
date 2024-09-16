#!/bin/sh

set -ex

go build

rm -f *.mid

for x in *.json; do
	./midiconverser -i "$x"
done
