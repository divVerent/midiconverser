# MIDI Converser

Converter and player for MIDI files to play hymns in church on electronic
organs.

Runs on Linux with ALSA and an USB MIDI device (or, for local testing, with
FluidSynth).

## Usage

### Generate Files For a Hymn

1. Write a JSON file. TODO: add details.

2. `go run main.go -i hymnnumber.json`

### Play a Hymn

1. `./player.sh hymnnumber`

2. When prompted, hit any key to proceed with the hymn. It waits twice at a
   fermata (once to stop the note, and a second time to resume the hymn), and
   once before each verse. Watch the conductor at these points.

3. If needed, press `Ctrl-C` to exit (preferably during a break). Otherwise it
   will exit at the configured number of verses.

### Play arbitrary hymns in random order (for prelude).

1. `./player.sh`

2. Press `Ctrl-C` to exit (preferably during a break).

## Where to Get Hymns

To be described later.

## License

This software is released under the [GPL 3.0](COPYING.md).
