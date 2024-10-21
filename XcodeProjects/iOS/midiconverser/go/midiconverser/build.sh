#!/bin/sh

set -ex

export CGO_ENABLED=1
export CGO_CPPFLAGS=-DNDEBUG
export CGO_CFLAGS='-g0 -O3'
export CGO_CXXFLAGS='-g0 -O3'
export CGO_LDFLAGS='-g0 -O3'
export GOOS=ios

ebiver=$(go mod graph | grep ^github.com/hajimehoshi/ebiten/v2@ | cut -d @ -f 2 | cut -d ' ' -f 1 | uniq)
go mod tidy
go get github.com/hajimehoshi/ebiten/v2/cmd/ebitenmobile@$ebiver
trap 'go mod tidy' EXIT

go run github.com/hajimehoshi/ebiten/v2/cmd/ebitenmobile bind \
	-target ios \
	-o midiconverser.xcframework \
	-iosversion 12.0 \
	-tags 'embed age' \
	-gcflags=all=-dwarf=false \
	-ldflags=all='-s -w' \
	-trimpath \
	-a \
	github.com/divVerent/midiconverser/XcodeProjects/iOS/midiconverser/go/midiconverser
