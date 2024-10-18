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
EBITENUI_PLAYER_DEPS = cmd/ebitenui_player/vfs.zip
else
EBITENUI_PLAYER_DEPS = cmd/ebitenui_player/vfs.zip.age
endif
endif

all: ebitenui_player$(GOEXE) process$(GOEXE) textui_player$(GOEXE)

clean:
	$(RM) internal/version/version.txt ebitenui_player$(GOEXE) process$(GOEXE) textui_player$(GOEXE) cmd/ebitenui_player/vfs.zip cmd/ebitenui_player/vfs.zip.age wasm_exec.js ebitenui_player.service-worker.js

internal/version/version.txt:
	echo $$(git describe --always --long --match 'v*.*' --exclude 'v*.*.*' HEAD)-$$(git log -n 1 --pretty=format:%cd --date=format:%Y%m%d HEAD) > "$@"

ebitenui_player$(GOEXE): internal/version/version.txt $(EBITENUI_PLAYER_DEPS)
	$(GO) build $(GO_FLAGS) -o $@ ./cmd/ebitenui_player

process$(GOEXE): internal/version/version.txt
	$(GO) build $(GO_FLAGS) -o $@ ./cmd/process

textui_player$(GOEXE): internal/version/version.txt
	$(GO) build $(GO_FLAGS) -o $@ ./cmd/textui_player

cmd/ebitenui_player/vfs.zip: ../midi
	set -ex; \
	pwd=$(PWD); \
	cd ../midi; \
	echo $$(git describe --always --long --match 'v*.*' --exclude 'v*.*.*' HEAD)-$$(git log -n 1 --pretty=format:%cd --date=format:%Y%m%d HEAD) > version.txt; \
	cd hymns-extra; \
	./build.sh; \
	cd ..; \
	$(7ZA) "$$pwd"/$@ version.txt *.yml */*.mid

cmd/ebitenui_player/vfs.zip.age: cmd/ebitenui_player/vfs.zip
	$(AGE) --encrypt --passphrase -o $@ $<

wasm_exec.js:
	cp "$(shell cd / && $(GO) env GOROOT)"/misc/wasm/wasm_exec.js .

ebitenui_player.service-worker.js: ebitenui_player.workbox-config.js ebitenui_player.wasm wasm_exec.js
	$(WORKBOX) generateSW ebitenui_player.workbox-config.js
