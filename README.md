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
   * `fermata_extend_beats`: number of extra beats to hold fermata notes (default: 1). Affects only the pre-arranged MIDI outputs.
   * `fermata_rest_beats`: number of rest beats after a fermata (default: 1). Affects only the pre-arranged MIDI outputs.
   * `rest_between_verses_beats`: number of beats to wait between verses (default: 1). Affects only the pre-arranged MIDI outputs.
   * `prelude_player_repeat`: number of times each hymn will be repeated in the prelude player (default: 2).
   * `prelude_player_sleep_sec`: number of seconds between hymns in the prelude player (default: 2).

1. Write a JSON file like the following:

   ```
   {
     "input_file": "../hymns/27.mid",
     "fermatas": [
       "22.2+1/4"
     ],
     "prelude": [
       {
         "begin": "1.1",
         "end": "5.1"
       },
       {
         "begin": "29.1",
         "end": "33.1"
       }
     ],
     "num_verses": 4,
     "qpm_override": 86
   }
   ```

   with the following keys:

   * `input_file`: MIDI file to read.
   * `fermatas`: list of positions of fermatas (default: empty); this should point _inside_ the note to hold (ideally halfway).
   * `prelude`: list of begin/end positions for the prelude (default: empty); the end positions are exclusive and thus should be the beat where the next non-prelude portion begins. The last item can point behind the last bar.
   * `num_verses`: number of verses of this hymn (default: 1).
   * `qpm_override`: replacement value for tempo in quarter notes per minute, if nonzero (default: 0).
   * `bpm_factor`: tempo factor to adjust the input (default: 1.0). Only really makes sense to use when not using `qpm_override`.
   * `max_adjust`: maximum number of MIDI ticks to adjust positions by (default: 64).
   * `keep_event_order`: try to retain event order within a tick (default: false).

   whereas a "position" is a quoted string of the form:

   * `bar.beat` to specify an exact beat
   * `bar.beat+num/denom` to specify a position between two beats; the fraction is the fraction of the next beat to use

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
