module github.com/divVerent/midiconverser

go 1.24.2

require (
	filippo.io/age v1.2.1
	github.com/ebitenui/ebitenui v0.7.2
	github.com/hajimehoshi/ebiten/v2 v2.8.8
	github.com/jeandeaual/go-locale v0.0.0-20250612000132-0ef82f21eade
	gitlab.com/gomidi/midi/v2 v2.3.16
	golang.org/x/image v0.31.0
	golang.org/x/term v0.35.0
	golang.org/x/text v0.29.0
	gopkg.in/yaml.v3 v3.0.1
)

require (
	c2sp.org/CCTV/age v0.0.0-20250126162742-ac53b9fb362b // indirect
	github.com/ebitengine/gomobile v0.0.0-20250923094054-ea854a63cce1 // indirect
	github.com/ebitengine/hideconsole v1.0.0 // indirect
	github.com/ebitengine/purego v0.9.0 // indirect
	github.com/go-text/typesetting v0.3.0 // indirect
	github.com/go-text/typesetting-utils v0.0.0-20250317161857-4bc07585f84e // indirect
	github.com/hajimehoshi/bitmapfont/v3 v3.2.1 // indirect
	github.com/jezek/xgb v1.1.1 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/pierrec/lz4/v4 v4.1.22 // indirect
	github.com/rogpeppe/go-internal v1.14.1 // indirect
	golang.org/x/crypto v0.42.0 // indirect
	golang.org/x/exp v0.0.0-20250911091902-df9299821621 // indirect
	golang.org/x/sync v0.17.0 // indirect
	golang.org/x/sys v0.36.0 // indirect
)

// Pin ebitenui for now due to API change.
replace github.com/ebitenui/ebitenui => github.com/ebitenui/ebitenui v0.6.2
