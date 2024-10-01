package main

import (
	"bytes"
	"flag"
	"fmt"
	"image/color"
	"log"
	"math"
	"os"
	"time"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/ebitenui/ebitenui"
	"github.com/ebitenui/ebitenui/image"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"gitlab.com/gomidi/midi/v2/drivers"

	"github.com/divVerent/midiconverser/internal/file"
	"github.com/divVerent/midiconverser/internal/player"
)

var (
	c    = flag.String("c", "config.yml", "config file name (YAML)")
	port = flag.String("port", "", "regular expression to match the preferred output port")
	i    = flag.String("i", "", "when set, just play this file then exit")
)

type playerUI struct {
	ui *ebitenui.UI

	backend *player.Backend
	outPort drivers.Out
	uiState player.UIState

	currentlyPlaying *widget.Label
	statusLabel      *widget.Label
	status           *widget.Label
	tempoLabel       *widget.Label
	tempo            *widget.Slider
	playbackLabel    *widget.Label
	playback         *widget.ProgressBar
	verseLabel       *widget.Label
	moreVerses       *widget.Button
	fewerVerses      *widget.Button
	stop             *widget.Button
	prompt           *widget.Button

	tempoLastChange time.Time
	loopErr         error
}

func main() {
	flag.Parse()

	ebiten.SetWindowSize(720, 1280)
	ebiten.SetWindowTitle("MIDI Converser - graphical player")
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetWindowClosingHandled(true)

	var playerUI playerUI
	err := playerUI.initBackend()
	if err != nil {
		log.Print(err)
		os.Exit(1)
	}
	defer playerUI.shutdownBackend()
	playerUI.initUI()
	err = ebiten.RunGame(&playerUI)
	if err != nil {
		log.Print(err)
		os.Exit(1)
	}
}

func (p *playerUI) initBackend() error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %v", err)
	}
	fsys := os.DirFS(cwd)

	p.outPort, err = player.FindBestPort(*port)
	if err != nil {
		return fmt.Errorf("could not find MIDI port: %w", err)
	}
	log.Printf("Picked output port: %v", p.outPort)

	err = p.outPort.Open()
	if err != nil {
		return fmt.Errorf("could not open MIDI port %v: %w", p.outPort, err)
	}

	config, err := file.ReadConfig(fsys, *c)
	if err != nil {
		return fmt.Errorf("failed to read config: %w", err)
	}

	p.backend = player.NewBackend(&player.Options{
		FSys:     fsys,
		Config:   config,
		OutPort:  p.outPort,
		PlayOnly: *i,
	})

	go func() {
		p.loopErr = p.backend.Loop()
		close(p.backend.UIStates)
	}()

	return nil
}

func (p *playerUI) shutdownBackend() {
	close(p.backend.Commands)
	for {
		_, ok := <-p.backend.UIStates
		if !ok {
			break
		}
	}
	p.backend = nil

	p.outPort.Close()
}

func (p *playerUI) initUI() {
	font, err := text.NewGoTextFaceSource(bytes.NewReader(goregular.TTF))
	if err != nil {
		log.Fatal(err)
	}
	fontFace := &text.GoTextFace{
		Source: font,
		Size:   32,
	}

	rootContainer := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(image.NewNineSliceColor(color.White)),
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(1),
			widget.GridLayoutOpts.Spacing(16, 16),
			widget.GridLayoutOpts.Padding(widget.NewInsetsSimple(16)),
			widget.GridLayoutOpts.Stretch([]bool{true}, []bool{false, false, true}),
		)),
	)

	p.ui = &ebitenui.UI{
		Container: rootContainer,
	}

	mainContainer := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(image.NewNineSliceColor(color.White)),
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(2),
			widget.GridLayoutOpts.Spacing(16, 16),
			widget.GridLayoutOpts.Stretch([]bool{false, true}, []bool{false, false}),
		)),
	)
	rootContainer.AddChild(mainContainer)

	labelColors := &widget.LabelColor{
		Idle:     color.Black,
		Disabled: color.Gray{Y: 128},
	}
	buttonTextColor := &widget.ButtonTextColor{
		Idle:    color.White,
		Hover:   color.Gray{Y: 64},
		Pressed: color.Black,
	}
	buttonImage := &widget.ButtonImage{
		Idle:    image.NewNineSliceColor(color.Black),
		Hover:   image.NewNineSliceColor(color.Gray{Y: 192}),
		Pressed: image.NewNineSliceColor(color.White),
	}
	sliderTrackImage := &widget.SliderTrackImage{
		Idle:  image.NewNineSliceColor(color.Gray{Y: 128}),
		Hover: image.NewNineSliceColor(color.Gray{Y: 160}),
	}
	sliderButtonImage := &widget.ButtonImage{
		Idle:    image.NewNineSliceColor(color.Black),
		Hover:   image.NewNineSliceColor(color.Gray{Y: 64}),
		Pressed: image.NewNineSliceColor(color.Gray{Y: 192}),
	}
	progressTrackImage := &widget.ProgressBarImage{
		Idle:     image.NewNineSliceColor(color.Gray{Y: 128}),
		Disabled: image.NewNineSliceColor(color.Gray{Y: 192}),
	}
	progressImage := &widget.ProgressBarImage{
		Idle:     image.NewNineSliceColor(color.Black),
		Disabled: image.NewNineSliceColor(color.Gray{Y: 192}),
	}

	currentlyPlayingLabel := widget.NewLabel(
		widget.LabelOpts.Text("Currently Playing: ", fontFace, labelColors),
	)
	mainContainer.AddChild(currentlyPlayingLabel)
	p.currentlyPlaying = widget.NewLabel(
		widget.LabelOpts.Text("...", fontFace, labelColors),
	)
	mainContainer.AddChild(p.currentlyPlaying)

	p.statusLabel = widget.NewLabel(
		widget.LabelOpts.Text("Status: ", fontFace, labelColors),
	)
	mainContainer.AddChild(p.statusLabel)
	p.status = widget.NewLabel(
		widget.LabelOpts.Text("...", fontFace, labelColors),
	)
	mainContainer.AddChild(p.status)

	// TODO add a control for prelude tags.

	p.tempoLabel = widget.NewLabel(
		widget.LabelOpts.Text("Tempo: ...", fontFace, labelColors),
	)
	mainContainer.AddChild(p.tempoLabel)
	p.tempo = widget.NewSlider(
		widget.SliderOpts.MinMax(50, 200),
		widget.SliderOpts.Images(sliderTrackImage, sliderButtonImage),
		widget.SliderOpts.ChangedHandler(p.tempoChanged),
		widget.SliderOpts.PageSizeFunc(func() int {
			return 1
		}),
	)
	mainContainer.AddChild(p.tempo)

	p.verseLabel = widget.NewLabel(
		widget.LabelOpts.Text("Verse: ...", fontFace, labelColors),
	)
	mainContainer.AddChild(p.verseLabel)

	versesContainer := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(image.NewNineSliceColor(color.White)),
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(3),
			widget.GridLayoutOpts.Spacing(16, 16),
			widget.GridLayoutOpts.Stretch([]bool{false, false, true}, []bool{false}),
		)),
	)
	mainContainer.AddChild(versesContainer)

	p.fewerVerses = widget.NewButton(
		widget.ButtonOpts.Text("-", fontFace, buttonTextColor),
		widget.ButtonOpts.Image(buttonImage),
		widget.ButtonOpts.TextPadding(widget.Insets{Left: 8, Right: 8}),
		widget.ButtonOpts.ClickedHandler(p.fewerVersesClicked),
	)
	versesContainer.AddChild(p.fewerVerses)

	p.moreVerses = widget.NewButton(
		widget.ButtonOpts.Text("+", fontFace, buttonTextColor),
		widget.ButtonOpts.Image(buttonImage),
		widget.ButtonOpts.TextPadding(widget.Insets{Left: 8, Right: 8}),
		widget.ButtonOpts.ClickedHandler(p.moreVersesClicked),
	)
	versesContainer.AddChild(p.moreVerses)

	p.playbackLabel = widget.NewLabel(
		widget.LabelOpts.Text("Playback: ...", fontFace, labelColors),
	)
	mainContainer.AddChild(p.playbackLabel)
	p.playback = widget.NewProgressBar(
		widget.ProgressBarOpts.Images(progressTrackImage, progressImage),
	)
	mainContainer.AddChild(p.playback)

	playContainer := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(image.NewNineSliceColor(color.White)),
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(3),
			widget.GridLayoutOpts.Spacing(16, 16),
			widget.GridLayoutOpts.Stretch([]bool{true, false, false}, []bool{false}),
		)),
	)
	rootContainer.AddChild(playContainer)

	playHymn := widget.NewButton(
		widget.ButtonOpts.Text("Play Hymn...", fontFace, buttonTextColor),
		widget.ButtonOpts.Image(buttonImage),
		widget.ButtonOpts.TextPadding(widget.NewInsetsSimple(8)),
		widget.ButtonOpts.ClickedHandler(p.playHymnClicked),
	)
	playContainer.AddChild(playHymn)

	playPrelude := widget.NewButton(
		widget.ButtonOpts.Text("Play Prelude", fontFace, buttonTextColor),
		widget.ButtonOpts.Image(buttonImage),
		widget.ButtonOpts.TextPadding(widget.NewInsetsSimple(8)),
		widget.ButtonOpts.ClickedHandler(p.playPreludeClicked),
	)
	playContainer.AddChild(playPrelude)

	p.stop = widget.NewButton(
		widget.ButtonOpts.Text("Stop", fontFace, buttonTextColor),
		widget.ButtonOpts.Image(buttonImage),
		widget.ButtonOpts.TextPadding(widget.NewInsetsSimple(8)),
		widget.ButtonOpts.ClickedHandler(p.stopClicked),
	)
	playContainer.AddChild(p.stop)

	p.prompt = widget.NewButton(
		widget.ButtonOpts.Text("b", fontFace, buttonTextColor),
		widget.ButtonOpts.Image(buttonImage),
		widget.ButtonOpts.TextPadding(widget.NewInsetsSimple(8)),
		widget.ButtonOpts.ClickedHandler(p.promptClicked),
	)
	rootContainer.AddChild(p.prompt)
}

func (p *playerUI) playHymnClicked(args *widget.ButtonClickedEventArgs) {
	fmt.Println("click")
}

func (p *playerUI) playPreludeClicked(args *widget.ButtonClickedEventArgs) {
	p.backend.Commands <- player.Command{
		PlayPrelude: true,
	}
}

func (p *playerUI) stopClicked(args *widget.ButtonClickedEventArgs) {
	p.backend.Commands <- player.Command{
		Exit: true,
	}
}

func (p *playerUI) promptClicked(args *widget.ButtonClickedEventArgs) {
	p.backend.Commands <- player.Command{
		Answer: true,
	}
}

func (p *playerUI) tempoChanged(args *widget.SliderChangedEventArgs) {
	p.backend.Commands <- player.Command{
		Tempo: float64(args.Current) / 100.0,
	}
	p.tempoLastChange = time.Now()
}

func (p *playerUI) fewerVersesClicked(args *widget.ButtonClickedEventArgs) {
	p.backend.Commands <- player.Command{
		NumVerses: p.uiState.NumVerses - 1,
	}
}

func (p *playerUI) moreVersesClicked(args *widget.ButtonClickedEventArgs) {
	p.backend.Commands <- player.Command{
		NumVerses: p.uiState.NumVerses + 1,
	}
}

// updateUI updates all widgets to current playback state.
func (p *playerUI) updateWidgets() {
	var np string
	if p.uiState.PlayPrelude {
		np = fmt.Sprintf("%v (prelude player)", p.uiState.CurrentFile)
	} else if p.uiState.PlayOne != "" && p.uiState.Playing {
		np = fmt.Sprintf("%v (%v)", p.uiState.CurrentFile, p.uiState.CurrentPart)
	} else if p.uiState.PlayOne != "" {
		np = p.uiState.CurrentFile
	}
	p.currentlyPlaying.Label = np

	if p.uiState.Err != nil {
		p.statusLabel.Label = "Error:"
		p.status.Label = fmt.Sprint(p.uiState.Err)
		p.statusLabel.GetWidget().Visibility = widget.Visibility_Show
		p.status.GetWidget().Visibility = widget.Visibility_Show
	} else if p.uiState.CurrentMessage != "" {
		p.statusLabel.Label = "Status:"
		p.status.Label = p.uiState.CurrentMessage
		p.statusLabel.GetWidget().Visibility = widget.Visibility_Show
		p.status.GetWidget().Visibility = widget.Visibility_Show
	} else {
		p.statusLabel.GetWidget().Visibility = widget.Visibility_Hide_Blocking
		p.status.GetWidget().Visibility = widget.Visibility_Hide_Blocking
	}

	if time.Since(p.tempoLastChange) > time.Second {
		p.tempo.Current = int(math.Round(100.0 * p.uiState.Tempo))
	}
	p.tempoLabel.Label = fmt.Sprintf("Tempo: %d%%", p.tempo.Current)

	if p.uiState.NumVerses > 0 {
		p.verseLabel.Label = fmt.Sprintf("Verse: %d/%d", p.uiState.Verse+1, p.uiState.NumVerses)
		p.verseLabel.GetWidget().Visibility = widget.Visibility_Show
		p.fewerVerses.GetWidget().Visibility = widget.Visibility_Show
		p.moreVerses.GetWidget().Visibility = widget.Visibility_Show
		p.fewerVerses.GetWidget().Disabled = p.uiState.NumVerses <= 1
		p.moreVerses.GetWidget().Disabled = p.uiState.NumVerses >= 10
	} else {
		p.verseLabel.GetWidget().Visibility = widget.Visibility_Hide_Blocking
		p.fewerVerses.GetWidget().Visibility = widget.Visibility_Hide_Blocking
		p.moreVerses.GetWidget().Visibility = widget.Visibility_Hide_Blocking
		p.fewerVerses.GetWidget().Disabled = true
		p.moreVerses.GetWidget().Disabled = true
	}

	if p.uiState.Playing {
		p.playbackLabel.Label = "Playing:"
		p.playbackLabel.GetWidget().Disabled = false
		p.playback.Min = 0
		p.playback.Max = 1000000
		p.playback.SetCurrent(int(math.Round(1000000 * p.uiState.ActualPlaybackFraction())))
		p.playback.GetWidget().Disabled = false
		p.stop.GetWidget().Disabled = false
	} else if p.uiState.CurrentFile != "" {
		p.playbackLabel.Label = "Waiting:"
		p.playbackLabel.GetWidget().Disabled = false
		p.playback.GetWidget().Disabled = true
		p.stop.GetWidget().Disabled = false
	} else {
		p.playbackLabel.Label = "Stopped"
		p.playbackLabel.GetWidget().Disabled = true
		p.playback.GetWidget().Disabled = true
		p.stop.GetWidget().Disabled = true
	}

	if p.uiState.Prompt != "" {
		p.prompt.Text().Label = p.uiState.Prompt
		p.prompt.GetWidget().Visibility = widget.Visibility_Show
		p.prompt.GetWidget().Disabled = false
	} else {
		p.prompt.GetWidget().Visibility = widget.Visibility_Hide_Blocking
		p.prompt.GetWidget().Disabled = true
	}
}

func (p *playerUI) Update() error {
	// Refresh UI state.
updateLoop:
	for {
		select {
		case ui, ok := <-p.backend.UIStates:
			if !ok {
				log.Printf("UI closed")
				// UI channel was closed.
				if p.loopErr != nil {
					return p.loopErr
				}
				return player.QuitError
			}
			p.uiState = ui
		default:
			// All done.
			break updateLoop
		}
	}

	if ebiten.IsWindowBeingClosed() {
		p.backend.Commands <- player.Command{
			Quit: true,
		}
	}

	p.updateWidgets()
	p.ui.Update()
	return nil
}

func (p *playerUI) Draw(screen *ebiten.Image) {
	p.ui.Draw(screen)
}

func (p *playerUI) Layout(outsideWidth int, outsideHeight int) (int, int) {
	return outsideWidth, outsideHeight
}
