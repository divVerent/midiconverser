#!/bin/sh

set -ex

for step in clean ''; do
	make ${step:-all}
	if [ -d ../midi ]; then
		GOOS=windows GOARCH=386 CGO_ENABLED=1 CC=i686-w64-mingw32-gcc CXX=i686-w64-mingw32-g++ BUILDTAGS='embed age' make ${step:-ebitenui_player.exe}
		GOOS=js GOARCH=wasm BUILDTAGS='embed age' make ${step:-ebitenui_player.service-worker.js}
	fi
done
