#!/bin/sh

fs_path=$1

set -ex

files='
	ebitenui_player.exe
	ebitenui_player.html
	ebitenui_player.manifest.json
	ebitenui_player.svg
	ebitenui_player.wasm
	wasm_exec.js
	ebitenui_player.service-worker.js
'

for f in $files; do
	cp -v "$f" "$fs_path/$f"
done
