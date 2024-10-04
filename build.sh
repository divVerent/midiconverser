#!/bin/sh

set -ex

go build ./cmd/ebitenui_player
go build ./cmd/process
go build ./cmd/textui_player

CGO_ENABLED=1 GOOS=windows GOARCH=386 CC=i686-w64-mingw32-gcc CXX=i686-w64-mingw32-g++ go build -ldflags=all=-s -ldflags=all=-w -ldflags=all=-H=windowsgui -a -trimpath -o ebitenui_player.exe ./cmd/ebitenui_player

if [ -d ../midi ]; then
	rm -rf cmd/ebitenui_player/vfs
	mkdir cmd/ebitenui_player/vfs
	{
		cd ../midi
		tar cf - *.yml */*.mid
	} | {
		cd cmd/ebitenui_player/vfs
		tar xvf -
	}

	# Optimize the wasm binary for size.
	GOOS=js GOARCH=wasm go build -ldflags=all=-s -a -trimpath -gcflags=all=-B -gcflags=all=-dwarf=false -gcflags=all=-l -o ebitenui_player.wasm ./cmd/ebitenui_player

	cp "$(cd / && GOOS=js GOARCH=wasm go env GOROOT)"/misc/wasm/wasm_exec.js .
	sw-precache --config=ebitenui_player.sw-precache-config.js --verbose
	mv service-worker.js ebitenui_player.sw-precache.js
fi
