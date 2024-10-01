# MIDI Converser

Converter and player for MIDI files to play hymns in church on
electronic organs.

Runs on Linux with ALSA and an USB MIDI device (or, for local testing,
with FluidSynth).

## Installation

    sudo apt install golang                # For running the MIDI generator.
    sudo apt install alsa-utils figlet yq  # For using the included player.
    sudo apt install fluidsynth            # For playing with local speakers.
    go build

## Usage

### Generate Files For a Hymn

1.  If not done yet, write a YAML file `config.yml` like the following
    (written for the LD-34 organ):

        channel: 2
        melody_channel: 1
        melody_track_name_re: ^(Melody|Soprano|Voice|Voice 1)$
        bass_channel: 3
        bass_track_name_re: ^(Bass|Baritone)$
        fermatas_in_prelude: true
        fermatas_in_postlude: true

    with the following keys:

    -   `bpm_factor`: tempo factor as desired (default: 1.0).
    -   `channel`: MIDI channel (1-16) to map all notes to, or 0 to not
        remap (default). When using an organ, map this to the great
        manual.
    -   `hold_redundant_notes`: `true` to keep redundant notes playing,
        `false` to restart them (default).
    -   `fermata_extend_beats`: number of extra beats to hold fermata
        notes (default: 1). Affects only the pre-arranged MIDI outputs.
    -   `fermata_rest_beats`: number of rest beats after a fermata
        (default: 1). Affects only the pre-arranged MIDI outputs.
    -   `rest_between_verses_beats`: number of beats to wait between
        verses (default: 1). Affects only the pre-arranged MIDI outputs.
    -   `prelude_player_repeat`: number of times each hymn will be
        repeated in the prelude player (default: 2).
    -   `prelude_player_sleep_sec`: number of seconds between hymns in
        the prelude player (default: 2).
    -   `melody_track_name_re`: partial-match regular expression that
        melody track names should match (default: unset).
    -   `melody_channel`: additional channel for melody notes (default:
        unset). When using an organ, map this to the swell manual. This
        is basically the "melody coupler" feature some organs have.
    -   `bass_track_name_re`: partial-match regular expression that bass
        track names should match (default: unset).
    -   `bass_channel`: additional channel for bass notes (default:
        unset). When using an organ, map this to the pedal. This is
        basically the "bass coupler" feature some organs have.
    -   `fermatas_in_prelude`: interpret fermata instructions when
        generating the prelude (default: false).
    -   `fermatas_in_postlude`: interpret fermata instructions when
        generating the postlude (default: false).

2.  Write a YAML file like the following:

        input_file: ../hymns/27.mid
        fermatas:
          - "22.2+1/4"
        prelude:
          - begin: "1.1"
            end: "5.1"
          - begin: "29.1"
            end: "33.1"
        num_verses: 4
        qpm_override: 86

    with the following keys:

    -   `input_file`: MIDI file to read.
    -   `fermatas`: list of positions of fermatas (default: empty); this
        should point *inside* the note to hold (ideally halfway).
    -   `prelude`: list of begin/end positions for the prelude (default:
        empty); the end positions are exclusive and thus should be the
        beat where the next non-prelude portion begins. The last item
        can point behind the last bar.
    -   `num_verses`: number of verses of this hymn (default: 1).
    -   `postlude`: list of begin/end positions for the postlude
        (default: empty); the end positions are exclusive and thus
        should be the beat where the next non-prelude portion begins.
        The last item can point behind the last bar. Rarely ever needed.
    -   `qpm_override`: replacement value for tempo in quarter notes per
        minute, if nonzero (default: 0).
    -   `bpm_factor`: tempo factor to adjust the input (default: 1.0).
        Only really makes sense to use when not using `qpm_override`.
    -   `max_adjust`: maximum number of MIDI ticks to adjust positions
        by (default: 64).
    -   `keep_event_order`: try to retain event order within a tick
        (default: false).
    -   `melody_tracks`: list of track indexes (zero-based) to map to
        melody, overriding global settings (default: unset).
    -   `bass_tracks`: list of track indexes (zero-based) to map to
        bass, overriding global settings (default: unset).
    -   `fermatas_in_prelude`: interpret fermata instructions when
        generating the prelude (default: same as config).
    -   `fermatas_in_postlude`: interpret fermata instructions when
        generating the postlude (default: same as config).
    -   `input_file_sha256`: SHA-256 checksum of the input MIDI file
        content (optional; can be auto filled in when passing
        `-add_checksum`).

    whereas a "position" is a quoted string of the form:

    -   `bar.beat` to specify an exact beat
    -   `bar.beat+num/denom` to specify a position between two beats;
        the fraction is the fraction of the next beat to use

3.  `go run main.go -i hymnnumber.yml`

### Prepare for Playing

1.  Connect a MIDI device via USB, or launch FluidSynth as follows:

        fluidsynth -f fluidsynth.conf

    A sample `fluidsynth.conf` to sort of emulate the LD-34 organ:

        # Swell: loud reed organ.
        prog 0 20
        cc 0 7 100
        # Great: medium church organ.
        prog 1 19
        cc 1 7 90
        # Pedal: soft drawbar organ.
        prog 2 16
        cc 2 7 80

2.  Test that the desired output device exists:

        aplaymidi -l

### Play a Hymn

1.  `./player.sh hymnnumber`

2.  When prompted, hit any key to proceed with the hymn. It waits twice
    at a fermata (once to stop the note, and a second time to resume the
    hymn), and once before each verse. Watch the conductor at these
    points.

3.  If needed, press `Ctrl-C` to exit (preferably during a break).
    Otherwise it will exit at the configured number of verses.

### Play arbitrary hymns in random order (for prelude before the meeting).

1.  `./player.sh`

2.  Press `Ctrl-C` to exit (preferably during a break).

## Where to Get Hymns

To be described later.

## License

This software is released under the [GPL 3.0](COPYING.md).
