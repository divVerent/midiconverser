#!/bin/sh

set -ex

go build ./cmd/ebitenui_player
go build ./cmd/process
go build ./cmd/textui_player
