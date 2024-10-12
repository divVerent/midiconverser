#!/bin/sh

set -ex

version() {
	desc=$(git describe --always --long --match 'v*.*' --exclude 'v*.*.*' HEAD)
	date=$(git log -n 1 --pretty=format:%cd --date=format:%Y%m%d HEAD)
	echo "$desc-$date"
}

version > internal/version/version.txt

go build ./cmd/ebitenui_player
go build ./cmd/process
go build ./cmd/textui_player

EXTRA_LDFLAGS=

# Standard flags for release build.
go_build_release() {
	go build -a -ldflags=all="-s -w $EXTRA_LDFLAGS" -gcflags=all='-dwarf=false' -trimpath "$@"
}

# Special flags for even smaller binaries.
go_build_embedrelease() {
	go build -a -ldflags=all="-s -w $EXTRA_LDFLAGS" -gcflags=all='-B -dwarf=false' -trimpath -tags embed "$@"
}

win32() {
	(
		export CGO_ENABLED=1
		export GOOS=windows
		export GOARCH=386
		export CC=i686-w64-mingw32-gcc
		export CXX=i686-w64-mingw32-g++
		export EXTRA_LDFLAGS='-H=windowsgui -linkmode=external -extldflags=-static'
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

win32 go_build_release -o ebitenui_player_nodata.exe ./cmd/ebitenui_player

if [ -d ../midi ]; then
	rm -rf cmd/ebitenui_player/vfs
	mkdir cmd/ebitenui_player/vfs
	(
		cd ../midi
		version > version.txt
		(
			cd hymns-extra
			./build.sh
		)
		tar cf - version.txt *.yml */*.mid
	) | (
		cd cmd/ebitenui_player/vfs
		tar xvf -
	)

	win32 go_build_embedrelease -o ebitenui_player.exe ./cmd/ebitenui_player
	wasm go_build_embedrelease -o ebitenui_player.wasm ./cmd/ebitenui_player

	cp "$(cd / && GOOS=js GOARCH=wasm go env GOROOT)"/misc/wasm/wasm_exec.js .
	workbox generateSW ebitenui_player.workbox-config.js --verbose
fi
