#!/bin/sh

set -ex

go build ./cmd/ebitenui_player
go build ./cmd/process
go build ./cmd/textui_player

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
	GOOS=js GOARCH=wasm go build -ldflags=-s -ldflags=-w -a -trimpath -o ebitenui_player.wasm ./cmd/ebitenui_player
	cp "$(cd / && GOOS=js GOARCH=wasm go env GOROOT)"/misc/wasm/wasm_exec.js .
fi
