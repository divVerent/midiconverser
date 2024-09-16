#!/bin/sh

set -ex

go build

for x in *.json; do
	./midiconverser -i "$x"
done
