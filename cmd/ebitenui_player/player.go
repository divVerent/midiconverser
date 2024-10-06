package main

import (
	"bytes"
	"cmp"
	"errors"
	"flag"
	"fmt"
	go_image "image"
	"image/color"
	"io/fs"
	"log"
	"math"
	"os"
	"reflect"
	"runtime/debug"
	"slices"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/ebitenui/ebitenui"
	"github.com/ebitenui/ebitenui/image"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"gitlab.com/gomidi/midi/v2"
	"gitlab.com/gomidi/midi/v2/drivers"

	"github.com/divVerent/midiconverser/internal/file"
	"github.com/divVerent/midiconverser/internal/player"
	"github.com/divVerent/midiconverser/internal/processor"
)

var (
	c    = flag.String("c", "config.yml", "config file name (YAML)")
	port = flag.String("port", "", "regular expression to match the preferred output port")
	i    = flag.String("i", "", "when set, just play this file then exit")
)

type playerUI struct {
	ui *ebitenui.UI

	config  *processor.Config
	backend *player.Backend
	outPort drivers.Out
	uiState player.UIState

	hymnsAny    []any
	channelsAny []any
	tagsAny     []any
	outPorts    map[int]drivers.Out
	outPortsAny []any
	font        *text.GoTextFaceSource

	width, height int
	scale         float64

	rootContainer               *widget.Container
	currentlyPlaying            *widget.Label
	statusLabel                 *widget.Label
	status                      *widget.Label
	playbackLabel               *widget.Label
	playback                    *widget.ProgressBar
	tempoLabel                  *widget.Label
	tempo                       *widget.Slider
	verseLabel                  *widget.Label
	moreVerses                  *widget.Button
	fewerVerses                 *widget.Button
	stop                        *widget.Button
	prompt                      *widget.Button
	hymnsWindow                 *widget.Window
	hymnList                    *widget.List
	preludeWindow               *widget.Window
	preludeTagList              *widget.List
	settingsWindow              *widget.Window
	settingsOutPort             *widget.List
	settingsChannel             *widget.ListComboButton
	settingsMelodyChannel       *widget.ListComboButton
	settingsBassChannel         *widget.ListComboButton
	settingsHoldRedundantNotes  *widget.Checkbox
	settingsTempo               *widget.Slider
	settingsPreludePlayerRepeat *widget.Slider
	settingsPreludePlayerSleep  *widget.Slider
	settingsFermatasInPrelude   *widget.Checkbox

	prevTempo          float64
	loopErr            error
	hymnsWindowOpen    bool
	preludeWindowOpen  bool
	settingsWindowOpen bool
	prevPreludeTags    map[string]bool
}

func Main() error {
	flag.Parse()

	w, h := 360, 800
	ebiten.SetWindowSize(w, h)
	ebiten.SetWindowTitle("MIDI Converser - graphical player")
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetWindowClosingHandled(true)

	fsys, err := openFS()
	if err != nil {
		return fmt.Errorf("failed to open FS: %v", err)
	}

	p := playerUI{
		width:  w,
		height: h,
	}

	err = p.initHymnsList(fsys)
	if err != nil {
		return err
	}

	p.initChannelsList()

	err = p.initBackend(fsys)
	if err != nil {
		return err
	}
	defer p.shutdownBackend()

	err = p.initUI()
	if err != nil {
		return err
	}

	return ebiten.RunGame(&p)
}

func main() {
	flag.Parse()
	err := Main()
	if errors.Is(err, player.SigIntError) {
		os.Exit(127)
	}
	if err != nil && !errors.Is(err, player.QuitError) {
		log.Printf("Exiting due to: %v.", err)
		os.Exit(1)
	}
}

func copyConfigOverrideFields(from, to *processor.Config) {
	// Copy just the fields the GUI can change.
	// Prevents accidents.
	to.Channel = from.Channel
	to.MelodyChannel = from.MelodyChannel
	to.BassChannel = from.BassChannel
	to.HoldRedundantNotes = from.HoldRedundantNotes
	to.BPMFactor = from.BPMFactor
	to.PreludePlayerRepeat = from.PreludePlayerRepeat
	to.PreludePlayerSleepSec = from.PreludePlayerSleepSec
	to.FermatasInPrelude = from.FermatasInPrelude
}

func (p *playerUI) initBackend(fsys fs.FS) error {
	var err error
	p.outPort, err = player.FindBestPort(*port)
	if err != nil {
		log.Printf("Could not find MIDI port: %v - continuning without; playing will fail.", err)
	}
	log.Printf("Picked output port: %v.", p.outPort)

	p.config, err = loadConfig(fsys, *c)
	if err != nil {
		return fmt.Errorf("failed to read config: %w", err)
	}

	err = loadConfigOverride(*c, p.config)
	if err != nil {
		log.Printf("Failed to load config override: %v.", err)
	}

	p.backend = player.NewBackend(&player.Options{
		FSys:     fsys,
		Config:   p.config,
		OutPort:  p.outPort,
		PlayOnly: *i,
	})

	go func() {
		p.loopErr = p.backend.Loop()
		p.backend.Close()
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
}

func listHymns(fsys fs.FS) ([]string, []string, error) {
	all, err := fs.Glob(fsys, "*.yml")
	if err != nil {
		return nil, nil, fmt.Errorf("glob: %w", err)
	}

	var hymns []string
	tagsMap := map[string]bool{}
	for _, f := range all {
		options, err := file.ReadOptions(fsys, f)
		if err != nil {
			continue
		}
		hymns = append(hymns, f)
		for _, t := range options.Tags {
			tagsMap[t] = true
		}
	}

	slices.SortFunc(hymns, func(a, b string) int {
		aNum, bNum := 0, 0
		fmt.Sscanf(a, "%d", &aNum)
		fmt.Sscanf(b, "%d", &bNum)
		if aNum != bNum {
			return cmp.Compare(aNum, bNum)
		}
		return cmp.Compare(a, b)
	})

	var tags []string
	for t := range tagsMap {
		tags = append(tags, t)
	}
	slices.Sort(tags)

	return hymns, tags, nil
}

func (p *playerUI) initHymnsList(fsys fs.FS) error {
	hymns, tags, err := listHymns(fsys)
	if err != nil {
		return err
	}
	p.hymnsAny = make([]any, 0, len(hymns))
	for _, h := range hymns {
		p.hymnsAny = append(p.hymnsAny, h)
	}
	p.tagsAny = make([]any, 0, len(tags))
	for _, h := range tags {
		p.tagsAny = append(p.tagsAny, h)
	}
	return nil
}

func (p *playerUI) initChannelsList() {
	p.channelsAny = []any{
		0,
		1,
		2,
		3,
		4,
		5,
		6,
		7,
		8,
		9,
		// 10 skipped (percussion channel)
		11,
		12,
		13,
		14,
		15,
		16,
	}
}

func channelNameFunc(e any) string {
	ch := e.(int)
	if ch == 0 {
		return "unset"
	}
	return fmt.Sprintf("#%d", ch)
}

func (p *playerUI) initOutPortsList() {
	if p.outPorts != nil && len(p.outPorts) == 0 {
		// This is a rescan after it already was empty once.
		rescanMIDI()
	}
	p.outPortsAny = nil
	p.outPorts = map[int]drivers.Out{}
	for _, port := range midi.GetOutPorts() {
		p.outPortsAny = append(p.outPortsAny, port.Number())
		p.outPorts[port.Number()] = port
	}
	if len(p.outPortsAny) == 0 {
		p.outPortsAny = append(p.outPortsAny, nil)
	}
	return
}

func (p *playerUI) outPortNameFunc(e any) string {
	if e == nil {
		return "none"
	}
	return p.outPorts[e.(int)].String()
}

func (p *playerUI) tagNameFunc(e any) string {
	tag := e.(string)
	state, found := p.prevPreludeTags[tag]
	if !found {
		return tag
	}
	if state {
		return fmt.Sprintf("%v (requested)", tag)
	}
	return fmt.Sprintf("%v (avoided)", tag)
}

func (p *playerUI) initUI() error {
	var err error
	p.font, err = text.NewGoTextFaceSource(bytes.NewReader(goregular.TTF))
	if err != nil {
		return err
	}
	p.rootContainer = widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(image.NewNineSliceColor(color.White)),
		widget.ContainerOpts.Layout(widget.NewAnchorLayout()),
	)
	p.ui = &ebitenui.UI{
		Container: p.rootContainer,
	}
	return nil
}

func newImageColor(size int, c color.Color) *ebiten.Image {
	img := ebiten.NewImage(size, size)
	img.Fill(c)
	return img
}

func version() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "(no BuildInfo)"
	}
	for _, s := range info.Settings {
		if s.Key == "vcs.revision" {
			return s.Value
		}
	}
	return "(no vcs.revision)"
}

func (p *playerUI) recreateUI() {
	fontSize := 4.0 * p.scale
	smallFontSize := 2.0 * p.scale
	spacing := int(math.Round(3 * p.scale))
	listSliderSize := int(math.Round(8 * p.scale))
	buttonInsets := int(math.Round(p.scale))
	checkSize := int(math.Round(3 * p.scale))

	titleBarHeight := int(fontSize + 2*float64(buttonInsets))
	portListHeight := int(titleBarHeight * 3)

	fontFace := &text.GoTextFace{
		Source: p.font,
		Size:   fontSize,
	}
	smallFontFace := &text.GoTextFace{
		Source: p.font,
		Size:   smallFontSize,
	}

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
	scrollContainerImage := &widget.ScrollContainerImage{
		Idle:     image.NewNineSliceColor(color.Gray{Y: 192}),
		Disabled: image.NewNineSliceColor(color.Gray{Y: 192}),
		Mask:     image.NewNineSliceColor(color.Gray{Y: 192}),
	}
	listEntryColor := &widget.ListEntryColor{
		Selected:                   color.White,
		Unselected:                 color.Black,
		SelectedBackground:         color.Black,
		SelectingBackground:        color.White,
		SelectingFocusedBackground: color.Gray{Y: 192},
		SelectedFocusedBackground:  color.Black,
		FocusedBackground:          color.White,
		DisabledUnselected:         color.Black,
		DisabledSelected:           color.White,
		DisabledSelectedBackground: color.Black,
	}

	checkboxGraphicImage := &widget.CheckboxGraphicImage{
		Unchecked: &widget.ButtonImageImage{Idle: newImageColor(checkSize, color.NRGBA{R: 128, G: 128, B: 128, A: 32})},
		Checked:   &widget.ButtonImageImage{Idle: newImageColor(checkSize, color.NRGBA{R: 128, G: 128, B: 128, A: 255})},
		Greyed:    &widget.ButtonImageImage{Idle: newImageColor(checkSize, color.Alpha{A: 0})},
	}

	// Rebuild the rootContainer.

	p.rootContainer.RemoveChildren()

	mainContainer := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(1),
			widget.GridLayoutOpts.Spacing(spacing, spacing),
			widget.GridLayoutOpts.Padding(widget.NewInsetsSimple(spacing)),
			widget.GridLayoutOpts.Stretch([]bool{true}, []bool{false, false, false, false, true}),
		)),
		widget.ContainerOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.AnchorLayoutData{
				StretchHorizontal: true,
				StretchVertical:   true,
			})),
	)
	p.rootContainer.AddChild(mainContainer)

	versionLabel := widget.NewLabel(
		widget.LabelOpts.Text(fmt.Sprintf("Version: %s", version()), smallFontFace, labelColors),
		widget.LabelOpts.TextOpts(
			widget.TextOpts.Position(widget.TextPositionEnd, widget.TextPositionCenter),
		),
	)
	mainContainer.AddChild(versionLabel)

	tableContainer := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(2),
			widget.GridLayoutOpts.Spacing(spacing, spacing),
			widget.GridLayoutOpts.Stretch([]bool{false, true}, []bool{false}),
		)),
	)
	mainContainer.AddChild(tableContainer)

	currentlyPlayingLabel := widget.NewLabel(
		widget.LabelOpts.Text("Now Playing: ", fontFace, labelColors),
	)
	tableContainer.AddChild(currentlyPlayingLabel)
	p.currentlyPlaying = widget.NewLabel(
		widget.LabelOpts.Text("...", fontFace, labelColors),
	)
	tableContainer.AddChild(p.currentlyPlaying)

	p.statusLabel = widget.NewLabel(
		widget.LabelOpts.Text("Status: ", fontFace, labelColors),
	)
	tableContainer.AddChild(p.statusLabel)
	p.status = widget.NewLabel(
		widget.LabelOpts.Text("...", fontFace, labelColors),
	)
	tableContainer.AddChild(p.status)

	p.playbackLabel = widget.NewLabel(
		widget.LabelOpts.Text("Playback: ...", fontFace, labelColors),
	)
	tableContainer.AddChild(p.playbackLabel)
	p.playback = widget.NewProgressBar(
		widget.ProgressBarOpts.Images(progressTrackImage, progressImage),
	)
	tableContainer.AddChild(p.playback)

	// TODO add a control for prelude tags.

	p.tempoLabel = widget.NewLabel(
		widget.LabelOpts.Text("Tempo: ...", fontFace, labelColors),
	)
	tableContainer.AddChild(p.tempoLabel)
	p.tempo = widget.NewSlider(
		widget.SliderOpts.MinMax(50, 200),
		widget.SliderOpts.Images(sliderTrackImage, sliderButtonImage),
		widget.SliderOpts.ChangedHandler(p.tempoChanged),
		widget.SliderOpts.PageSizeFunc(func() int {
			return 1
		}),
	)
	p.prevTempo = -1 // Force refresh.
	tableContainer.AddChild(p.tempo)

	p.verseLabel = widget.NewLabel(
		widget.LabelOpts.Text("Verse: ...", fontFace, labelColors),
	)
	tableContainer.AddChild(p.verseLabel)

	versesContainer := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(3),
			widget.GridLayoutOpts.Spacing(spacing, spacing),
			widget.GridLayoutOpts.Stretch([]bool{false, false, true}, []bool{false}),
		)),
	)
	tableContainer.AddChild(versesContainer)

	p.fewerVerses = widget.NewButton(
		widget.ButtonOpts.Text("-", fontFace, buttonTextColor),
		widget.ButtonOpts.Image(buttonImage),
		widget.ButtonOpts.TextPadding(widget.Insets{Left: buttonInsets, Right: buttonInsets}),
		widget.ButtonOpts.ClickedHandler(p.fewerVersesClicked),
	)
	versesContainer.AddChild(p.fewerVerses)

	p.moreVerses = widget.NewButton(
		widget.ButtonOpts.Text("+", fontFace, buttonTextColor),
		widget.ButtonOpts.Image(buttonImage),
		widget.ButtonOpts.TextPadding(widget.Insets{Left: buttonInsets, Right: buttonInsets}),
		widget.ButtonOpts.ClickedHandler(p.moreVersesClicked),
	)
	versesContainer.AddChild(p.moreVerses)

	playContainer := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(3),
			widget.GridLayoutOpts.Spacing(spacing, spacing),
			widget.GridLayoutOpts.Stretch([]bool{true, false, false}, []bool{false}),
		)),
	)
	mainContainer.AddChild(playContainer)

	selectHymn := widget.NewButton(
		widget.ButtonOpts.Text("Hymn...", fontFace, buttonTextColor),
		widget.ButtonOpts.Image(buttonImage),
		widget.ButtonOpts.TextPadding(widget.NewInsetsSimple(buttonInsets)),
		widget.ButtonOpts.ClickedHandler(p.selectHymnClicked),
	)
	playContainer.AddChild(selectHymn)

	selectPrelude := widget.NewButton(
		widget.ButtonOpts.Text("Prelude...", fontFace, buttonTextColor),
		widget.ButtonOpts.Image(buttonImage),
		widget.ButtonOpts.TextPadding(widget.NewInsetsSimple(buttonInsets)),
		widget.ButtonOpts.ClickedHandler(p.selectPreludeClicked),
	)
	playContainer.AddChild(selectPrelude)

	p.stop = widget.NewButton(
		widget.ButtonOpts.Text("Stop", fontFace, buttonTextColor),
		widget.ButtonOpts.Image(buttonImage),
		widget.ButtonOpts.TextPadding(widget.NewInsetsSimple(buttonInsets)),
		widget.ButtonOpts.ClickedHandler(p.stopClicked),
	)
	playContainer.AddChild(p.stop)

	settings := widget.NewButton(
		widget.ButtonOpts.Text("Settings...", fontFace, buttonTextColor),
		widget.ButtonOpts.Image(buttonImage),
		widget.ButtonOpts.TextPadding(widget.NewInsetsSimple(buttonInsets)),
		widget.ButtonOpts.ClickedHandler(p.settingsClicked),
	)
	mainContainer.AddChild(settings)

	p.prompt = widget.NewButton(
		widget.ButtonOpts.Text("b", fontFace, buttonTextColor),
		widget.ButtonOpts.Image(buttonImage),
		widget.ButtonOpts.TextPadding(widget.NewInsetsSimple(buttonInsets)),
		widget.ButtonOpts.ClickedHandler(p.promptClicked),
	)
	p.prompt.GetWidget().Disabled = true
	mainContainer.AddChild(p.prompt)

	// Rebuild the hymns window.

	hymnsWindowContainer := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(image.NewNineSliceColor(color.Gray{Y: 224})),
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(1),
			widget.GridLayoutOpts.Spacing(spacing, spacing),
			widget.GridLayoutOpts.Padding(widget.NewInsetsSimple(spacing)),
			widget.GridLayoutOpts.Stretch([]bool{true}, []bool{false, true, false}),
		)),
	)

	chooseHymnLabel := widget.NewLabel(
		widget.LabelOpts.Text("Choose Hymn: ", fontFace, labelColors),
	)
	hymnsWindowContainer.AddChild(chooseHymnLabel)

	p.hymnList = widget.NewList(
		widget.ListOpts.Entries(p.hymnsAny),
		widget.ListOpts.ScrollContainerOpts(
			widget.ScrollContainerOpts.Image(scrollContainerImage),
		),
		widget.ListOpts.SliderOpts(widget.SliderOpts.Images(sliderTrackImage, sliderButtonImage),
			widget.SliderOpts.MinHandleSize(listSliderSize),
		),
		widget.ListOpts.HideHorizontalSlider(),
		widget.ListOpts.EntryFontFace(fontFace),
		widget.ListOpts.EntryColor(listEntryColor),
		widget.ListOpts.EntryLabelFunc(func(e interface{}) string {
			return e.(string)
		}),
		widget.ListOpts.EntryTextPadding(widget.NewInsetsSimple(buttonInsets)),
	)
	hymnsWindowContainer.AddChild(p.hymnList)

	playHymn := widget.NewButton(
		widget.ButtonOpts.Text("Play Hymn", fontFace, buttonTextColor),
		widget.ButtonOpts.Image(buttonImage),
		widget.ButtonOpts.TextPadding(widget.NewInsetsSimple(buttonInsets)),
		widget.ButtonOpts.ClickedHandler(p.playHymnClicked),
	)
	hymnsWindowContainer.AddChild(playHymn)

	hymnsTitleContainer := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(image.NewNineSliceColor(color.Black)),
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(2),
			widget.GridLayoutOpts.Spacing(spacing, spacing),
			widget.GridLayoutOpts.Stretch([]bool{true, false}, []bool{true}),
		)),
	)
	hymnsTitle := widget.NewText(
		widget.TextOpts.Text("Play Hymn", fontFace, color.White),
		widget.TextOpts.Insets(widget.Insets{Left: buttonInsets, Right: buttonInsets}),
		widget.TextOpts.Position(widget.TextPositionStart, widget.TextPositionCenter),
	)
	hymnsTitleContainer.AddChild(hymnsTitle)
	hymnsCloseButton := widget.NewButton(
		widget.ButtonOpts.Text("X", fontFace, buttonTextColor),
		widget.ButtonOpts.Image(buttonImage),
		widget.ButtonOpts.TextPadding(widget.Insets{Left: buttonInsets, Right: buttonInsets}),
		widget.ButtonOpts.ClickedHandler(p.hymnsCloseClicked),
	)
	hymnsTitleContainer.AddChild(hymnsCloseButton)

	if p.hymnsWindowOpen {
		p.hymnsWindow.Close()
	}

	p.hymnsWindow = widget.NewWindow(
		widget.WindowOpts.Contents(hymnsWindowContainer),
		widget.WindowOpts.TitleBar(hymnsTitleContainer, titleBarHeight),
		widget.WindowOpts.Modal(),
		widget.WindowOpts.CloseMode(widget.NONE),
	)

	if p.hymnsWindowOpen {
		p.selectHymnClicked(nil)
	}

	// Rebuild the prelude window.
	preludeWindowContainer := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(image.NewNineSliceColor(color.Gray{Y: 224})),
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(1),
			widget.GridLayoutOpts.Spacing(spacing, spacing),
			widget.GridLayoutOpts.Padding(widget.NewInsetsSimple(spacing)),
			widget.GridLayoutOpts.Stretch([]bool{true}, []bool{false, true, false, false}),
		)),
	)

	chooseTagsLabel := widget.NewLabel(
		widget.LabelOpts.Text("Choose Tags: ", fontFace, labelColors),
	)
	preludeWindowContainer.AddChild(chooseTagsLabel)

	p.preludeTagList = widget.NewList(
		widget.ListOpts.Entries(p.tagsAny),
		widget.ListOpts.ScrollContainerOpts(
			widget.ScrollContainerOpts.Image(scrollContainerImage),
		),
		widget.ListOpts.SliderOpts(widget.SliderOpts.Images(sliderTrackImage, sliderButtonImage),
			widget.SliderOpts.MinHandleSize(listSliderSize),
		),
		widget.ListOpts.HideHorizontalSlider(),
		widget.ListOpts.EntryFontFace(fontFace),
		widget.ListOpts.EntryColor(listEntryColor),
		widget.ListOpts.EntryLabelFunc(p.tagNameFunc),
		widget.ListOpts.EntryTextPadding(widget.NewInsetsSimple(buttonInsets)),
	)
	preludeWindowContainer.AddChild(p.preludeTagList)

	tagActionContainer := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(3),
			widget.GridLayoutOpts.Spacing(spacing, spacing),
			widget.GridLayoutOpts.Stretch([]bool{false, true, false}, []bool{false}),
		)),
	)
	preludeWindowContainer.AddChild(tagActionContainer)

	avoidTag := widget.NewButton(
		widget.ButtonOpts.Text("Avoid", fontFace, buttonTextColor),
		widget.ButtonOpts.Image(buttonImage),
		widget.ButtonOpts.TextPadding(widget.NewInsetsSimple(buttonInsets)),
		widget.ButtonOpts.ClickedHandler(p.avoidTagClicked),
	)
	tagActionContainer.AddChild(avoidTag)

	ignoreTag := widget.NewButton(
		widget.ButtonOpts.Text("Ignore", fontFace, buttonTextColor),
		widget.ButtonOpts.Image(buttonImage),
		widget.ButtonOpts.TextPadding(widget.NewInsetsSimple(buttonInsets)),
		widget.ButtonOpts.ClickedHandler(p.ignoreTagClicked),
	)
	tagActionContainer.AddChild(ignoreTag)

	requestTag := widget.NewButton(
		widget.ButtonOpts.Text("Request", fontFace, buttonTextColor),
		widget.ButtonOpts.Image(buttonImage),
		widget.ButtonOpts.TextPadding(widget.NewInsetsSimple(buttonInsets)),
		widget.ButtonOpts.ClickedHandler(p.requestTagClicked),
	)
	tagActionContainer.AddChild(requestTag)

	playPrelude := widget.NewButton(
		widget.ButtonOpts.Text("Play Prelude", fontFace, buttonTextColor),
		widget.ButtonOpts.Image(buttonImage),
		widget.ButtonOpts.TextPadding(widget.NewInsetsSimple(buttonInsets)),
		widget.ButtonOpts.ClickedHandler(p.playPreludeClicked),
	)
	preludeWindowContainer.AddChild(playPrelude)

	preludeTitleContainer := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(image.NewNineSliceColor(color.Black)),
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(2),
			widget.GridLayoutOpts.Spacing(spacing, spacing),
			widget.GridLayoutOpts.Stretch([]bool{true, false}, []bool{true}),
		)),
	)
	preludeTitle := widget.NewText(
		widget.TextOpts.Text("Play Prelude", fontFace, color.White),
		widget.TextOpts.Insets(widget.Insets{Left: buttonInsets, Right: buttonInsets}),
		widget.TextOpts.Position(widget.TextPositionStart, widget.TextPositionCenter),
	)
	preludeTitleContainer.AddChild(preludeTitle)
	preludeCloseButton := widget.NewButton(
		widget.ButtonOpts.Text("X", fontFace, buttonTextColor),
		widget.ButtonOpts.Image(buttonImage),
		widget.ButtonOpts.TextPadding(widget.Insets{Left: buttonInsets, Right: buttonInsets}),
		widget.ButtonOpts.ClickedHandler(p.preludeCloseClicked),
	)
	preludeTitleContainer.AddChild(preludeCloseButton)

	if p.preludeWindowOpen {
		p.preludeWindow.Close()
	}

	p.preludeWindow = widget.NewWindow(
		widget.WindowOpts.Contents(preludeWindowContainer),
		widget.WindowOpts.TitleBar(preludeTitleContainer, titleBarHeight),
		widget.WindowOpts.Modal(),
		widget.WindowOpts.CloseMode(widget.NONE),
	)

	if p.preludeWindowOpen {
		p.selectPreludeClicked(nil)
	}

	// Rebuild the settings window.

	settingsWindowContainer := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(image.NewNineSliceColor(color.Gray{Y: 224})),
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(1),
			widget.GridLayoutOpts.Spacing(spacing, spacing),
			widget.GridLayoutOpts.Padding(widget.NewInsetsSimple(spacing)),
			widget.GridLayoutOpts.Stretch([]bool{true}, []bool{false, true, false, false}),
		)),
	)

	outPortLabel := widget.NewLabel(
		widget.LabelOpts.Text("MIDI Port: ", fontFace, labelColors),
	)
	settingsWindowContainer.AddChild(outPortLabel)
	p.settingsOutPort = widget.NewList(
		widget.ListOpts.ContainerOpts(widget.ContainerOpts.WidgetOpts(
			widget.WidgetOpts.MinSize(0, portListHeight),
		)),
		widget.ListOpts.Entries(p.outPortsAny),
		widget.ListOpts.ScrollContainerOpts(
			widget.ScrollContainerOpts.Image(scrollContainerImage),
		),
		widget.ListOpts.SliderOpts(widget.SliderOpts.Images(sliderTrackImage, sliderButtonImage),
			widget.SliderOpts.MinHandleSize(listSliderSize),
		),
		widget.ListOpts.HideHorizontalSlider(),
		widget.ListOpts.EntryFontFace(smallFontFace),
		widget.ListOpts.EntryColor(listEntryColor),
		widget.ListOpts.EntryLabelFunc(p.outPortNameFunc),
		widget.ListOpts.EntryTextPadding(widget.NewInsetsSimple(buttonInsets)),
		widget.ListOpts.EntryTextPosition(widget.TextPositionStart, widget.TextPositionCenter),
	)
	settingsWindowContainer.AddChild(p.settingsOutPort)

	settingsTableContainer := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(2),
			widget.GridLayoutOpts.Spacing(spacing, spacing),
			widget.GridLayoutOpts.Padding(widget.NewInsetsSimple(spacing)),
			widget.GridLayoutOpts.Stretch([]bool{false, true}, []bool{false}),
		)),
	)
	settingsWindowContainer.AddChild(settingsTableContainer)

	channelLabel := widget.NewLabel(
		widget.LabelOpts.Text("Main Channel: ", fontFace, labelColors),
	)
	settingsTableContainer.AddChild(channelLabel)
	p.settingsChannel = widget.NewListComboButton(
		widget.ListComboButtonOpts.SelectComboButtonOpts(
			widget.SelectComboButtonOpts.ComboButtonOpts(
				widget.ComboButtonOpts.ButtonOpts(
					widget.ButtonOpts.Image(buttonImage),
					widget.ButtonOpts.TextPadding(widget.Insets{Left: buttonInsets, Right: buttonInsets}),
					widget.ButtonOpts.Text("", fontFace, buttonTextColor),
				),
			),
		),
		widget.ListComboButtonOpts.ListOpts(
			widget.ListOpts.Entries(p.channelsAny),
			widget.ListOpts.ScrollContainerOpts(
				widget.ScrollContainerOpts.Image(scrollContainerImage),
			),
			widget.ListOpts.SliderOpts(widget.SliderOpts.Images(sliderTrackImage, sliderButtonImage),
				widget.SliderOpts.MinHandleSize(listSliderSize),
			),
			widget.ListOpts.HideHorizontalSlider(),
			widget.ListOpts.EntryFontFace(fontFace),
			widget.ListOpts.EntryColor(listEntryColor),
			widget.ListOpts.EntryTextPadding(widget.NewInsetsSimple(buttonInsets))),
		widget.ListComboButtonOpts.EntryLabelFunc(channelNameFunc, channelNameFunc),
	)
	settingsTableContainer.AddChild(p.settingsChannel)

	melodyChannelLabel := widget.NewLabel(
		widget.LabelOpts.Text("Melody Coupler: ", fontFace, labelColors),
	)
	settingsTableContainer.AddChild(melodyChannelLabel)
	p.settingsMelodyChannel = widget.NewListComboButton(
		widget.ListComboButtonOpts.SelectComboButtonOpts(
			widget.SelectComboButtonOpts.ComboButtonOpts(
				widget.ComboButtonOpts.ButtonOpts(
					widget.ButtonOpts.Image(buttonImage),
					widget.ButtonOpts.TextPadding(widget.Insets{Left: buttonInsets, Right: buttonInsets}),
					widget.ButtonOpts.Text("", fontFace, buttonTextColor),
				),
			),
		),
		widget.ListComboButtonOpts.ListOpts(
			widget.ListOpts.Entries(p.channelsAny),
			widget.ListOpts.ScrollContainerOpts(
				widget.ScrollContainerOpts.Image(scrollContainerImage),
			),
			widget.ListOpts.SliderOpts(widget.SliderOpts.Images(sliderTrackImage, sliderButtonImage),
				widget.SliderOpts.MinHandleSize(listSliderSize),
			),
			widget.ListOpts.HideHorizontalSlider(),
			widget.ListOpts.EntryFontFace(fontFace),
			widget.ListOpts.EntryColor(listEntryColor),
			widget.ListOpts.EntryTextPadding(widget.NewInsetsSimple(buttonInsets))),
		widget.ListComboButtonOpts.EntryLabelFunc(channelNameFunc, channelNameFunc),
	)
	settingsTableContainer.AddChild(p.settingsMelodyChannel)

	bassChannelLabel := widget.NewLabel(
		widget.LabelOpts.Text("Bass Coupler: ", fontFace, labelColors),
	)
	settingsTableContainer.AddChild(bassChannelLabel)
	p.settingsBassChannel = widget.NewListComboButton(
		widget.ListComboButtonOpts.SelectComboButtonOpts(
			widget.SelectComboButtonOpts.ComboButtonOpts(
				widget.ComboButtonOpts.ButtonOpts(
					widget.ButtonOpts.Image(buttonImage),
					widget.ButtonOpts.TextPadding(widget.Insets{Left: buttonInsets, Right: buttonInsets}),
					widget.ButtonOpts.Text("", fontFace, buttonTextColor),
				),
			),
		),
		widget.ListComboButtonOpts.ListOpts(
			widget.ListOpts.Entries(p.channelsAny),
			widget.ListOpts.ScrollContainerOpts(
				widget.ScrollContainerOpts.Image(scrollContainerImage),
			),
			widget.ListOpts.SliderOpts(widget.SliderOpts.Images(sliderTrackImage, sliderButtonImage),
				widget.SliderOpts.MinHandleSize(listSliderSize),
			),
			widget.ListOpts.HideHorizontalSlider(),
			widget.ListOpts.EntryFontFace(fontFace),
			widget.ListOpts.EntryColor(listEntryColor),
			widget.ListOpts.EntryTextPadding(widget.NewInsetsSimple(buttonInsets))),
		widget.ListComboButtonOpts.EntryLabelFunc(channelNameFunc, channelNameFunc),
	)
	settingsTableContainer.AddChild(p.settingsBassChannel)

	holdRedundantNotesLabel := widget.NewLabel(
		widget.LabelOpts.Text("Hold Redundant Notes: ", fontFace, labelColors),
	)
	settingsTableContainer.AddChild(holdRedundantNotesLabel)
	p.settingsHoldRedundantNotes = widget.NewCheckbox(
		widget.CheckboxOpts.ButtonOpts(
			widget.ButtonOpts.Image(buttonImage),
		),
		widget.CheckboxOpts.Image(checkboxGraphicImage),
	)
	settingsTableContainer.AddChild(p.settingsHoldRedundantNotes)

	settingsTempoLabel := widget.NewLabel(
		widget.LabelOpts.Text("Global Tempo: 100%", fontFace, labelColors),
	)
	settingsTableContainer.AddChild(settingsTempoLabel)
	p.settingsTempo = widget.NewSlider(
		widget.SliderOpts.MinMax(50, 200),
		widget.SliderOpts.Images(sliderTrackImage, sliderButtonImage),
		widget.SliderOpts.ChangedHandler(func(args *widget.SliderChangedEventArgs) {
			settingsTempoLabel.Label = fmt.Sprintf("Global Tempo: %d%%", args.Current)
		}),
		widget.SliderOpts.PageSizeFunc(func() int {
			return 1
		}),
	)
	p.settingsTempo.Current = 100
	settingsTableContainer.AddChild(p.settingsTempo)

	settingsPreludePlayerRepeatLabel := widget.NewLabel(
		widget.LabelOpts.Text("Prelude Repeats: 2", fontFace, labelColors),
	)
	settingsTableContainer.AddChild(settingsPreludePlayerRepeatLabel)
	p.settingsPreludePlayerRepeat = widget.NewSlider(
		widget.SliderOpts.MinMax(1, 5),
		widget.SliderOpts.Images(sliderTrackImage, sliderButtonImage),
		widget.SliderOpts.ChangedHandler(func(args *widget.SliderChangedEventArgs) {
			settingsPreludePlayerRepeatLabel.Label = fmt.Sprintf("Prelude Repeats: %d", p.settingsPreludePlayerRepeat.Current)
		}),
		widget.SliderOpts.PageSizeFunc(func() int {
			return 1
		}),
	)
	p.settingsPreludePlayerRepeat.Current = 2
	settingsTableContainer.AddChild(p.settingsPreludePlayerRepeat)

	settingsPreludePlayerSleepLabel := widget.NewLabel(
		widget.LabelOpts.Text("Prelude Sleep: 2.0s", fontFace, labelColors),
	)
	settingsTableContainer.AddChild(settingsPreludePlayerSleepLabel)
	p.settingsPreludePlayerSleep = widget.NewSlider(
		widget.SliderOpts.MinMax(5, 50),
		widget.SliderOpts.Images(sliderTrackImage, sliderButtonImage),
		widget.SliderOpts.ChangedHandler(func(args *widget.SliderChangedEventArgs) {
			settingsPreludePlayerSleepLabel.Label = fmt.Sprintf("Prelude Sleep: %.1fs", float64(args.Current)*0.1)
		}),
		widget.SliderOpts.PageSizeFunc(func() int {
			return 5
		}),
	)
	p.settingsPreludePlayerSleep.Current = 20
	settingsTableContainer.AddChild(p.settingsPreludePlayerSleep)

	fermatasInPreludeLabel := widget.NewLabel(
		widget.LabelOpts.Text("Fermatas Everywhere: ", fontFace, labelColors),
	)
	settingsTableContainer.AddChild(fermatasInPreludeLabel)
	p.settingsFermatasInPrelude = widget.NewCheckbox(
		widget.CheckboxOpts.ButtonOpts(
			widget.ButtonOpts.Image(buttonImage),
		),
		widget.CheckboxOpts.Image(checkboxGraphicImage),
	)
	settingsTableContainer.AddChild(p.settingsFermatasInPrelude)

	applySettings := widget.NewButton(
		widget.ButtonOpts.Text("Apply", fontFace, buttonTextColor),
		widget.ButtonOpts.Image(buttonImage),
		widget.ButtonOpts.TextPadding(widget.NewInsetsSimple(buttonInsets)),
		widget.ButtonOpts.ClickedHandler(p.applySettingsClicked),
	)
	settingsWindowContainer.AddChild(applySettings)

	settingsTitleContainer := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(image.NewNineSliceColor(color.Black)),
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(2),
			widget.GridLayoutOpts.Spacing(spacing, spacing),
			widget.GridLayoutOpts.Stretch([]bool{true, false}, []bool{true}),
		)),
	)
	settingsTitle := widget.NewText(
		widget.TextOpts.Text("Settings", fontFace, color.White),
		widget.TextOpts.Insets(widget.Insets{Left: buttonInsets, Right: buttonInsets}),
		widget.TextOpts.Position(widget.TextPositionStart, widget.TextPositionCenter),
	)
	settingsTitleContainer.AddChild(settingsTitle)
	settingsCloseButton := widget.NewButton(
		widget.ButtonOpts.Text("X", fontFace, buttonTextColor),
		widget.ButtonOpts.Image(buttonImage),
		widget.ButtonOpts.TextPadding(widget.Insets{Left: buttonInsets, Right: buttonInsets}),
		widget.ButtonOpts.ClickedHandler(p.settingsCloseClicked),
	)
	settingsTitleContainer.AddChild(settingsCloseButton)

	if p.settingsWindowOpen {
		p.settingsWindow.Close()
	}

	p.settingsWindow = widget.NewWindow(
		widget.WindowOpts.Contents(settingsWindowContainer),
		widget.WindowOpts.TitleBar(settingsTitleContainer, titleBarHeight),
		widget.WindowOpts.Modal(),
		widget.WindowOpts.CloseMode(widget.NONE),
	)

	if p.settingsWindowOpen {
		p.settingsClicked(nil)
	}
}

func (p *playerUI) stopClicked(args *widget.ButtonClickedEventArgs) {
	p.ui.ClearFocus()
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

func (p *playerUI) selectHymnClicked(args *widget.ButtonClickedEventArgs) {
	w := p.width - 32
	h := p.height - 32
	x := (p.width - w) / 2
	y := (p.height - h) / 2
	r := go_image.Rect(x, y, x+w, y+h)
	p.hymnsWindow.SetLocation(r)
	p.ui.AddWindow(p.hymnsWindow)
	p.hymnsWindowOpen = true
}

func (p *playerUI) playHymnClicked(args *widget.ButtonClickedEventArgs) {
	p.stop.Focus(true)
	p.hymnsWindow.Close()
	p.hymnsWindowOpen = false
	e, ok := p.hymnList.SelectedEntry().(string)
	if !ok {
		log.Printf("No hymn selected.")
		return
	}
	p.backend.Commands <- player.Command{
		PlayOne: e,
	}
}

func (p *playerUI) hymnsCloseClicked(args *widget.ButtonClickedEventArgs) {
	p.ui.ClearFocus()
	p.hymnsWindow.Close()
	p.hymnsWindowOpen = false
}

func (p *playerUI) selectPreludeClicked(args *widget.ButtonClickedEventArgs) {
	w := p.width - 32
	_, tH := p.preludeWindow.TitleBar.PreferredSize()
	_, cH := p.preludeWindow.Contents.PreferredSize()
	h := p.height - 32
	if h > tH+cH {
		h = tH + cH
	}
	x := (p.width - w) / 2
	y := (p.height - h) / 2
	r := go_image.Rect(x, y, x+w, y+h)
	p.preludeWindow.SetLocation(r)
	p.ui.AddWindow(p.preludeWindow)
	p.preludeWindowOpen = true
}

func (p *playerUI) playPreludeClicked(args *widget.ButtonClickedEventArgs) {
	p.stop.Focus(true)
	p.preludeWindow.Close()
	p.preludeWindowOpen = false
	p.backend.Commands <- player.Command{
		PlayPrelude: true,
	}
}

func (p *playerUI) preludeCloseClicked(args *widget.ButtonClickedEventArgs) {
	p.ui.ClearFocus()
	p.preludeWindow.Close()
	p.preludeWindowOpen = false
}

func (p *playerUI) avoidTagClicked(args *widget.ButtonClickedEventArgs) {
	tag, ok := p.preludeTagList.SelectedEntry().(string)
	if !ok {
		log.Printf("No tag selected.")
		return
	}
	tags := player.CopyPreludeTags(p.prevPreludeTags)
	tags[tag] = false
	p.backend.Commands <- player.Command{
		PreludeTags: tags,
	}
}

func (p *playerUI) ignoreTagClicked(args *widget.ButtonClickedEventArgs) {
	tag, ok := p.preludeTagList.SelectedEntry().(string)
	if !ok {
		log.Printf("No tag selected.")
		return
	}
	tags := player.CopyPreludeTags(p.prevPreludeTags)
	delete(tags, tag)
	p.backend.Commands <- player.Command{
		PreludeTags: tags,
	}
}

func (p *playerUI) requestTagClicked(args *widget.ButtonClickedEventArgs) {
	tag, ok := p.preludeTagList.SelectedEntry().(string)
	if !ok {
		log.Printf("No tag selected.")
		return
	}
	tags := player.CopyPreludeTags(p.prevPreludeTags)
	tags[tag] = true
	p.backend.Commands <- player.Command{
		PreludeTags: tags,
	}
}

func (p *playerUI) settingsClicked(args *widget.ButtonClickedEventArgs) {
	p.initOutPortsList()
	p.settingsOutPort.SetEntries(p.outPortsAny)
	if p.outPort != nil {
		p.settingsOutPort.SetSelectedEntry(p.outPort.Number())
	}
	p.settingsChannel.SetSelectedEntry(p.config.Channel)
	p.settingsMelodyChannel.SetSelectedEntry(p.config.MelodyChannel)
	p.settingsBassChannel.SetSelectedEntry(p.config.BassChannel)
	if p.config.HoldRedundantNotes {
		p.settingsHoldRedundantNotes.SetState(widget.WidgetChecked)
	} else {
		p.settingsHoldRedundantNotes.SetState(widget.WidgetUnchecked)
	}
	p.settingsTempo.Current = int(math.Round(processor.WithDefault(p.config.BPMFactor, 1.0) * 100))
	p.settingsPreludePlayerRepeat.Current = processor.WithDefault(p.config.PreludePlayerRepeat, 2)
	p.settingsPreludePlayerSleep.Current = int(math.Round(processor.WithDefault(p.config.PreludePlayerSleepSec, 2.0) * 10))
	if p.config.FermatasInPrelude {
		p.settingsFermatasInPrelude.SetState(widget.WidgetChecked)
	} else {
		p.settingsFermatasInPrelude.SetState(widget.WidgetUnchecked)
	}

	w := p.width - 32
	_, tH := p.settingsWindow.TitleBar.PreferredSize()
	_, cH := p.settingsWindow.Contents.PreferredSize()
	h := p.height - 32
	if h > tH+cH {
		h = tH + cH
	}
	x := (p.width - w) / 2
	y := (p.height - h) / 2
	r := go_image.Rect(x, y, x+w, y+h)
	p.settingsWindow.SetLocation(r)
	p.ui.AddWindow(p.settingsWindow)
	p.settingsWindowOpen = true
}

func (p *playerUI) applySettingsClicked(args *widget.ButtonClickedEventArgs) {
	p.ui.ClearFocus()
	p.settingsWindow.Close()
	p.settingsWindowOpen = false
	nextPort := p.settingsOutPort.SelectedEntry()
	if nextPort != nil {
		port := p.outPorts[nextPort.(int)]
		if p.outPort == nil || port.Number() != p.outPort.Number() {
			p.outPort = port
			p.backend.Commands <- player.Command{
				OutPort: port,
			}
		}
	}
	p.config.Channel = p.settingsChannel.SelectedEntry().(int)
	p.config.MelodyChannel = p.settingsMelodyChannel.SelectedEntry().(int)
	p.config.BassChannel = p.settingsBassChannel.SelectedEntry().(int)
	p.config.HoldRedundantNotes = p.settingsHoldRedundantNotes.State() == widget.WidgetChecked
	p.config.BPMFactor = float64(p.settingsTempo.Current) * 0.01
	p.config.PreludePlayerRepeat = p.settingsPreludePlayerRepeat.Current
	p.config.PreludePlayerSleepSec = float64(p.settingsPreludePlayerSleep.Current) * 0.1
	p.config.FermatasInPrelude = p.settingsFermatasInPrelude.State() == widget.WidgetChecked
	saveConfigOverride(*c, p.config)
	p.backend.Commands <- player.Command{
		Config: p.config,
	}
}

func (p *playerUI) settingsCloseClicked(args *widget.ButtonClickedEventArgs) {
	p.ui.ClearFocus()
	p.settingsWindow.Close()
	p.settingsWindowOpen = false
}

// updateUI updates all widgets to current playback state.
func (p *playerUI) updateWidgets() {
	scale := math.Min(
		float64(p.width)/80,
		float64(p.height)/120,
	)
	if math.Abs(scale-p.scale) > 0.01 {
		p.scale = scale
		p.recreateUI()
	}

	p.currentlyPlaying.Label = p.uiState.CurrentFile

	if p.uiState.Err != nil {
		p.statusLabel.Label = "Error: "
		p.status.Label = fmt.Sprint(p.uiState.Err)
		p.statusLabel.GetWidget().Visibility = widget.Visibility_Show
		p.status.GetWidget().Visibility = widget.Visibility_Show
	} else if p.uiState.CurrentMessage != "" {
		p.statusLabel.Label = "Status: "
		p.status.Label = p.uiState.CurrentMessage
		p.statusLabel.GetWidget().Visibility = widget.Visibility_Show
		p.status.GetWidget().Visibility = widget.Visibility_Show
	} else {
		p.statusLabel.GetWidget().Visibility = widget.Visibility_Hide_Blocking
		p.status.GetWidget().Visibility = widget.Visibility_Hide_Blocking
	}

	if p.uiState.Playing {
		p.playbackLabel.Label = "Playing: "
		p.playbackLabel.GetWidget().Disabled = false
		p.playback.Min = 0
		p.playback.Max = 1000000
		p.playback.SetCurrent(int(math.Round(1000000 * p.uiState.ActualPlaybackFraction())))
		p.playback.GetWidget().Disabled = false
		p.stop.GetWidget().Disabled = false
	} else if p.uiState.CurrentFile != "" {
		p.playbackLabel.Label = "Waiting: "
		p.playbackLabel.GetWidget().Disabled = false
		p.playback.GetWidget().Disabled = true
		p.stop.GetWidget().Disabled = false
	} else {
		p.playbackLabel.Label = "Stopped"
		p.playbackLabel.GetWidget().Disabled = true
		p.playback.GetWidget().Disabled = true
		p.stop.GetWidget().Disabled = true
	}

	if p.uiState.Tempo != p.prevTempo {
		p.tempo.Current = int(math.Round(100.0 * p.uiState.Tempo))
		p.prevTempo = p.uiState.Tempo
	}
	p.tempoLabel.Label = fmt.Sprintf("Tempo: %d%%", p.tempo.Current)

	if p.uiState.NumVerses > 0 {
		postludeSuffix := ""
		if p.uiState.HavePostlude {
			postludeSuffix = "+P"
		}
		p.verseLabel.Label = fmt.Sprintf("Verse: %d/%d%s", p.uiState.Verse+1, p.uiState.NumVerses, postludeSuffix)
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

	if p.uiState.Prompt != "" {
		if p.prompt.GetWidget().Disabled {
			p.prompt.Focus(true)
		}
		p.prompt.Text().Label = p.uiState.Prompt
		p.prompt.GetWidget().Visibility = widget.Visibility_Show
		p.prompt.GetWidget().Disabled = false
	} else {
		p.prompt.GetWidget().Visibility = widget.Visibility_Hide_Blocking
		p.prompt.GetWidget().Disabled = true
	}

	if !reflect.DeepEqual(p.uiState.PreludeTags, p.prevPreludeTags) {
		p.prevPreludeTags = p.uiState.PreludeTags
		selected, ok := p.preludeTagList.SelectedEntry().(string)
		p.preludeTagList.SetEntries(p.tagsAny)
		if ok {
			p.preludeTagList.SetSelectedEntry(selected)
		}
	}
}

func (p *playerUI) Update() error {
	if ebiten.IsWindowBeingClosed() {
		p.backend.Commands <- player.Command{
			Quit: true,
		}
	}

updateLoop:
	for {
		select {
		case ui, ok := <-p.backend.UIStates:
			if !ok {
				log.Printf("UI closed.")
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

	p.updateWidgets()

	if p.outPort == nil && !p.settingsWindowOpen {
		p.settingsClicked(nil)
	}

	wakelockSet(p.uiState.PlayOne != "" || p.uiState.PlayPrelude)

	p.ui.Update()
	return nil
}

func (p *playerUI) Draw(screen *ebiten.Image) {
	p.ui.Draw(screen)
}

func (p *playerUI) Layout(outsideWidth int, outsideHeight int) (int, int) {
	p.width = outsideWidth
	p.height = outsideHeight
	return p.width, p.height
}
