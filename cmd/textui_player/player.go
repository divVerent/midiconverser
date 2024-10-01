package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"regexp"
	"slices"
	"strings"
	"time"

	"golang.org/x/term"

	"github.com/divVerent/midiconverser/internal/file"
	"github.com/divVerent/midiconverser/internal/player"
)

var (
	c    = flag.String("c", "config.yml", "config file name (YAML)")
	port = flag.String("port", "", "regular expression to match the preferred output port")
	i    = flag.String("i", "", "when set, just play this file then exit")
)

var (
	preludeRE = regexp.MustCompile(`^prelude$`)
	playRE    = regexp.MustCompile(`^play (\S+)$`)
	tagsRE    = regexp.MustCompile(`^tags((?: -?\w+)*)$`)
	tempoRE   = regexp.MustCompile(`^tempo ([\d.]+)$`)
	versesRE  = regexp.MustCompile(`^verses (\d+)$`)
	quitRE    = regexp.MustCompile(`^q(?:u(?:it?)?)?$`)
)

func processCommand(b *player.Backend, cmd []byte) error {
	if preludeRE.Match(cmd) {
		b.Commands <- player.Command{
			PlayPrelude: true,
		}
		return nil
	}
	if sub := playRE.FindSubmatch(cmd); sub != nil {
		filename := string(sub[1])
		_, err := os.Stat(filename)
		if err != nil {
			altName := filename + ".yml"
			_, err := os.Stat(altName)
			if err == nil {
				filename = altName
			}
		}
		b.Commands <- player.Command{
			PlayOne: filename,
		}
		return nil
	}
	if sub := tagsRE.FindSubmatch(cmd); sub != nil {
		b.Commands <- player.Command{
			PreludeTags: preludeTagsFromStr(string(sub[1])),
		}
		return nil
	}
	if sub := tempoRE.FindSubmatch(cmd); sub != nil {
		num := 0.0
		_, err := fmt.Sscanf(string(sub[1]), "%f", &num)
		if err != nil {
			return errors.New("failed to parse command: does not end with a number")
		}
		if num <= 0 {
			return errors.New("tempo must be positive")
		}
		b.Commands <- player.Command{
			Tempo: num,
		}
		return nil
	}
	if sub := versesRE.FindSubmatch(cmd); sub != nil {
		num := 0
		_, err := fmt.Sscanf(string(sub[1]), "%d", &num)
		if err != nil {
			return errors.New("failed to parse command: does not end with an integer")
		}
		if num < 1 {
			return errors.New("verse count must be positive")
		}
		b.Commands <- player.Command{
			NumVerses: num,
		}
		return nil
	}
	if quitRE.Match(cmd) {
		b.Commands <- player.Command{
			Quit: true,
		}
		return nil
	}
	return errors.New("unknown command")
}

func preludeTagsStr(tags map[string]bool) string {
	var keys []string
	for k := range tags {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		v := tags[k]
		if v {
			out = append(out, k)
		} else {
			out = append(out, "-"+k)
		}
	}
	return strings.Join(out, " ")
}

func preludeTagsFromStr(s string) map[string]bool {
	words := strings.Split(s, " ")
	tags := make(map[string]bool, len(words))
	for _, w := range words {
		if w == "" {
			continue
		}
		if w[0] == '-' {
			tags[w[1:]] = false
			continue
		}
		tags[w] = true
	}
	return tags
}

func textModeUI(b *player.Backend) error {
	defer close(b.Commands) // This will invariably cause failure when reading.

	if *i != "" {
		b.Commands <- player.Command{
			PlayOne: *i,
		}
	}

	stdinFD := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(stdinFD)
	if err != nil {
		return fmt.Errorf("cannot make terminal raw: %v", err)
	}
	defer term.Restore(stdinFD, oldState)

	stdin := make(chan byte)
	go func() {
		buf := make([]byte, 1)
		for {
			n, err := os.Stdin.Read(buf)
			if err != nil {
				log.Printf("error reading stdin: %v", err)
				close(stdin)
				return
			}
			if n == 0 {
				continue
			}
			stdin <- buf[0]
		}
	}()

	var ui player.UIState
	var ok bool
	inputMode := false
	var inputCommand []byte
	var commandErr error
	wasPlaying := false
	quitSent := false

	for {
		var np string
		if ui.PlayPrelude {
			np = fmt.Sprintf("%v (prelude player)", ui.CurrentFile)
		} else if ui.PlayOne != "" && ui.Playing {
			np = fmt.Sprintf("%v (%v)", ui.CurrentFile, ui.CurrentPart)
		} else if ui.PlayOne != "" {
			np = ui.CurrentFile
		}

		var bar string
		if ui.Playing {
			bar = " >>  "
			if ui.PlaybackLen > 0 {
				fReal := ui.ActualPlaybackFraction()
				for i := 0; i <= 73; i++ {
					f := float64(i) / 73
					if fReal >= f {
						bar += "#"
					} else {
						bar += "="
					}
				}
			}
			wasPlaying = true
		} else if ui.CurrentFile != "" {
			bar = " ||  =========================================================================="
			wasPlaying = true
		} else {
			bar = "[  ] --------------------------------------------------------------------------"
		}
		ifLine := func(b bool, s string) string {
			if !b {
				return ""
			}
			return s
		}
		lines := []string{
			"\033[m\033[2J\033[H\033[1;34mMIDI Converser - text mode player\033[m",
			"",
			ifLine(np != "", fmt.Sprintf("\033[1mCurrently Playing:\033[m %v", np)),
			ifLine(ui.CurrentMessage != "", fmt.Sprintf("\033[1mStatus:\033[m %v", ui.CurrentMessage)),
			"",
			ifLine(len(ui.PreludeTags) != 0, fmt.Sprintf("\033[1mPrelude tags:\033[m %v", preludeTagsStr(ui.PreludeTags))),
			ifLine(ui.Tempo != 0, fmt.Sprintf("\033[1mTempo:\033[m %.0f%%", 100*ui.Tempo)),
			ifLine(ui.NumVerses != 0, fmt.Sprintf("\033[1mVerse:\033[m %d/%d", ui.Verse+1, ui.NumVerses)),
			"",
			bar,
			"",
			ifLine(ui.Err != nil, fmt.Sprintf("\033[1;31mError:\033[0;31m %v\033[m", ui.Err)),
			ifLine(ui.Prompt != "", fmt.Sprintf("\033[1;33mPrompt: %v\033[m", ui.Prompt)),
			"",
			ifLine(commandErr != nil, fmt.Sprintf("\033[1;31mCommand Error:\033[0;31m %v\033[m", commandErr)),
			ifLine(inputMode, fmt.Sprintf("\033[1m:\033[m%s", inputCommand)),
		}
		os.Stderr.Write([]byte(strings.Join(lines, "\r\n")))

		select {
		case ui, ok = <-b.UIStates:
			if !ok {
				// UI channel was closed.
				return nil
			}
			if *i != "" {
				if !quitSent && (ui.Err != nil || (wasPlaying && ui.CurrentFile == "")) {
					b.Commands <- player.Command{
						Quit: true,
					}
					quitSent = true
				}
			}
			// Rest handled above.
		case ch := <-stdin:
			if inputMode {
				switch ch {
				case 0x08, 0x7F:
					if len(inputCommand) > 0 {
						inputCommand = inputCommand[:len(inputCommand)-1]
					}
				case 0x0A, 0x0D:
					if len(inputCommand) > 0 {
						err := processCommand(b, inputCommand)
						if err != nil {
							commandErr = fmt.Errorf("could not parse command %q: %v", inputCommand, err)
						}
					}
					inputCommand = inputCommand[:0]
					inputMode = false
				case 0x03:
					// Ctrl-C. Quit right away.
					b.Commands <- player.Command{
						Quit: true,
					}
				case 0x1B:
					inputMode = false
				default:
					if ch == ':' && len(inputCommand) == 0 {
						continue
					}
					inputCommand = append(inputCommand, ch)
				}
			} else {
				switch ch {
				case '+', '=', '.':
					// More tempo.
					t := ui.Tempo + 0.01
					if t > 2 {
						t = 2
					}
					b.Commands <- player.Command{
						Tempo: t,
					}
				case '-', '_', ',':
					// Less tempo.
					t := ui.Tempo - 0.01
					if t < 0.5 {
						t = 0.5
					}
					b.Commands <- player.Command{
						Tempo: t,
					}
				case 0x03:
					// Ctrl-C. Quit right away.
					b.Commands <- player.Command{
						Quit: true,
					}
				case 0x08, 0x7F:
					b.Commands <- player.Command{
						Exit: true,
					}
					commandErr = nil
				case 0x1B:
					commandErr = nil
				case ':':
					// Input mode during playback.
					commandErr = nil
					inputMode = true
				default:
					// "Any key".
					if ui.Prompt != "" {
						b.Commands <- player.Command{
							Answer: true,
						}
					}
				}
			}
		case <-time.After(50 * time.Millisecond):
			// At least 20 fps update.
		}
	}
	return nil
}

func Main() error {
	var err error
	outPort, err := player.FindBestPort(*port)
	if err != nil {
		return fmt.Errorf("could not find MIDI port: %w", err)
	}
	log.Printf("Picked output port: %v", outPort)

	err = outPort.Open()
	if err != nil {
		return fmt.Errorf("could not open MIDI port %v: %w", outPort, err)
	}
	defer outPort.Close()

	config, err := file.ReadConfig(*c)
	if err != nil {
		return fmt.Errorf("failed to read config: %w", err)
	}

	b := player.NewBackend(config, outPort)

	var loopErr error
	go func() {
		loopErr = b.Loop()
	}()

	err = textModeUI(b)
	if err != nil {
		return err
	}
	return loopErr
}

func main() {
	flag.Parse()
	err := Main()
	if errors.Is(err, player.SigIntError) || errors.Is(err, player.QuitError) {
		os.Exit(127)
	}
	if err != nil {
		log.Println(err)
		os.Exit(1)
	}
}
