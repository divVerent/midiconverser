#!/bin/sh

set -ex

go build ./cmd/ebitenui_player
go build ./cmd/process
go build ./cmd/textui_player
GOOS=js GOARCH=wasm go build -o ebitenui_player.wasm ./cmd/ebitenui_player
cp "$(cd / && GOOS=js GOARCH=wasm go env GOROOT)"/misc/wasm/wasm_exec.js .
