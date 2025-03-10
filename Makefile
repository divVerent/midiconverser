# General configuration.
GO ?= go
7ZA ?= 7za a -tzip -mx=9
AGE ?= age
WORKBOX ?= workbox

# Platform specific configuration.
GO_GCFLAGS ?= -dwarf=false
BUILDTAGS ?=
GO_FLAGS ?= -trimpath
GO_FLAGS += "-tags=$(BUILDTAGS)"
GO_FLAGS += "-gcflags=all=$(GO_GCFLAGS)"
GO_FLAGS += "-ldflags=all=$(GO_LDFLAGS)"

ifeq ($(shell $(GO) env GOOS),windows)
GOEXE ?= .exe
GO_LDFLAGS += -H=windowsgui -linkmode=external -extldflags=-static
else
ifeq ($(shell $(GO) env GOARCH),wasm)
GOEXE ?= .wasm
GO_LDFLAGS ?= -s -w
else
GOEXE ?=
GO_LDFLAGS ?= -s -w
endif
endif

ifeq ($(filter embed,$(BUILDTAGS)),)
EBITENUI_PLAYER_DEPS =
else
ifeq ($(filter age,$(BUILDTAGS)),)
EBITENUI_PLAYER_DEPS = internal/ebiplayer/vfs.zip
else
EBITENUI_PLAYER_DEPS = internal/ebiplayer/vfs.zip.age
endif
endif

SOURCES = $(shell find . -name \*.go)

all: ebitenui_player$(GOEXE) process$(GOEXE) textui_player$(GOEXE)
.PHONY: all

clean:
	$(RM) internal/version/version.txt ebitenui_player$(GOEXE) process$(GOEXE) textui_player$(GOEXE) internal/ebiplayer/vfs.zip internal/ebiplayer/vfs.zip.age wasm_exec.js ebitenui_player.service-worker.js XcodeProjects/iOS/midiconverser/go/midiconverser/midiconverser.xcframework
.PHONY: clean

internal/version/version.txt:
	echo $$(git describe --always --long --match 'v*.*' --exclude 'v*.*.*' HEAD)-$$(git log -n 1 --pretty=format:%cd --date=format:%Y%m%d HEAD) > "$@"

ebitenui_player$(GOEXE): internal/version/version.txt $(EBITENUI_PLAYER_DEPS) $(SOURCES)
	$(GO) build $(GO_FLAGS) -o $@ ./cmd/ebitenui_player

process$(GOEXE): internal/version/version.txt $(SOURCES)
	$(GO) build $(GO_FLAGS) -o $@ ./cmd/process

textui_player$(GOEXE): internal/version/version.txt $(SOURCES)
	$(GO) build $(GO_FLAGS) -o $@ ./cmd/textui_player

internal/ebiplayer/vfs.zip: ../midi
	set -ex; \
	pwd=$$PWD; \
	cd ../midi; \
	echo $$(git describe --always --long --match 'v*.*' --exclude 'v*.*.*' HEAD)-$$(git log -n 1 --pretty=format:%cd --date=format:%Y%m%d HEAD) > version.txt; \
	cd hymns-extra-abc; \
	./build.sh; \
	cd ..; \
	$(7ZA) "$$pwd"/$@ version.txt *.yml */*.mid

internal/ebiplayer/vfs.zip.age: internal/ebiplayer/vfs.zip
	$(AGE) --encrypt --passphrase -o $@ $<

wasm_exec.js:
	cp "$(shell cd / && $(GO) env GOROOT)"/lib/wasm/wasm_exec.js .

ebitenui_player.service-worker.js: ebitenui_player.workbox-config.js ebitenui_player.wasm wasm_exec.js
	$(WORKBOX) generateSW ebitenui_player.workbox-config.js

XcodeProjects/iOS/midiconverser/go/midiconverser/midiconverser.xcframework: internal/ebiplayer/vfs.zip.age
	set -ex; \
	cd XcodeProjects/iOS/midiconverser/go/midiconverser; \
	./build.sh

ios: internal/version/version.txt XcodeProjects/iOS/midiconverser/go/midiconverser/midiconverser.xcframework
	open XcodeProjects/iOS/midiconverser.xcodeproj
.PHONY: ios
