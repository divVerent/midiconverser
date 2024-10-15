#!/bin/sh

fs_path=$1
absolute_url=$2

set -ex

for f in ebitenui_player.exe ebitenui_player.service-worker.js ebitenui_player.svg ebitenui_player.wasm; do
	cp -v "$f" "$fs_path/$f"
done

for f in ebitenui_player.html ebitenui_player.manifest.json; do
	sed -e "s,ebitenui_player\\.,$absolute_url/$ebitenui_player.,g" < "$f" > "$fs_path/$f"
done
