#!/bin/sh

set -ex

go build ./cmd/ebitenui_player
go build ./cmd/process
go build ./cmd/textui_player

release_flags='
	-ldflags=all=-s
	-ldflags=all=-w
	-a
	-trimpath
'
embedrelease_flags='
	-tags embed
	-ldflags=all=-s
	-a
	-trimpath
	-gcflags=all=-B
	-gcflags=all=-dwarf=false
	-gcflags=all=-l
'

win32() {
	(
		export CGO_ENABLED=1
		export GOOS=windows
		export GOARCH=386
		export CC=i686-w64-mingw32-gcc
		export CXX=i686-w64-mingw32-g++
		"$@"
	)
}

wasm() {
	(
		export GOOS=js
		export GOARCH=wasm
		"$@"
	)
}

win32 go build $release_flags -ldflags=all=-H=windowsgui -o ebitenui_player_nodata.exe ./cmd/ebitenui_player

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

	win32 go build $embedrelease_flags -ldflags=all=-H=windowsgui -o ebitenui_player.exe ./cmd/ebitenui_player
	wasm go build $embedrelease_flags -o ebitenui_player.wasm ./cmd/ebitenui_player

	cp "$(cd / && GOOS=js GOARCH=wasm go env GOROOT)"/misc/wasm/wasm_exec.js .
	sw-precache --config=ebitenui_player.sw-precache-config.js --verbose
	mv service-worker.js ebitenui_player.sw-precache.js
fi
