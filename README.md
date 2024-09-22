# MIDI Converser

Converter and player for MIDI files to play hymns in church on electronic
organs.

Runs on Linux with ALSA and an USB MIDI device (or, for local testing, with
FluidSynth).

## Usage

### Generate Files For a Hymn

1. If not done yet, write a JSON file `config.json` like the following:

   ```
   {
     "channel": 2
   }
   ```

   with the following keys:

   * `bpm_factor`: tempo factor as desired (default: 1.0).
   * `channel`: MIDI channel (1-16) to map all notes to, or 0 to not remap (default).
   * `hold_redundant_notes`: `true` to keep redundant notes playing, `false` to restart them (default).
   * `fermata_extend`: number of extra beats to hold fermata notes (default: 1). Affects only the pre-arranged MIDI outputs.
   * `fermata_rest`: number of rest beats after a fermata (default: 1). Affects only the pre-arranged MIDI outputs.
   * `rest_between_verses`: number of beats to wait between verses (default: 1). Affects only the pre-arranged MIDI outputs.

1. Write a JSON file like the following:

   ```
   TODO
   ```

1. `go run main.go -i hymnnumber.json`

### Play a Hymn

1. `./player.sh hymnnumber`

1. When prompted, hit any key to proceed with the hymn. It waits twice at a
   fermata (once to stop the note, and a second time to resume the hymn), and
   once before each verse. Watch the conductor at these points.

1. If needed, press `Ctrl-C` to exit (preferably during a break). Otherwise it
   will exit at the configured number of verses.

### Play arbitrary hymns in random order (for prelude).

1. `./player.sh`

1. Press `Ctrl-C` to exit (preferably during a break).

## Where to Get Hymns

To be described later.

## License

This software is released under the [GPL 3.0](COPYING.md).
