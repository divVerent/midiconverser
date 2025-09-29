package ebiplayer

import (
	"bytes"
	"cmp"
	"fmt"
	"strings"
	"regexp"
	go_image "image"
	"image/color"
	"io/fs"
	"log"
	"path"
	"math"
	"reflect"
	"slices"

	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/text/language"

	"github.com/ebitenui/ebitenui"
	"github.com/ebitenui/ebitenui/image"
	"github.com/jeandeaual/go-locale"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"gitlab.com/gomidi/midi/v2"
	"gitlab.com/gomidi/midi/v2/drivers"

	"github.com/divVerent/midiconverser/internal/file"
	"github.com/divVerent/midiconverser/internal/player"
	"github.com/divVerent/midiconverser/internal/processor"
	"github.com/divVerent/midiconverser/internal/version"
)

type UI struct {
	ui *ebitenui.UI

	configFile    string
	inputFile     string
	requestedPort string

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

	rawWidth, rawHeight int
	insets              widget.Insets
	width, height       int
	scale               float64
	mustRecreateUI      bool

	rootContainer               *widget.Container
	currentlyPlaying            *widget.Label
	statusLabel                 *widget.Label
	status                      *widget.Label
	playbackLabel               *widget.Label
	playbackOrSkip              *widget.FlipBook
	skipVisible                 bool
	playback                    *widget.ProgressBar
	skip                        *widget.Button
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
	passwordWindow              *widget.Window
	password                    *widget.TextInput
	vKeys                       [][]*widget.Button
	vKeyMode                    int

	prevTempo          float64
	loopErr            error
	hymnsWindowOpen    bool
	preludeWindowOpen  bool
	settingsWindowOpen bool
	passwordWindowOpen bool
	prevPreludeTags    map[string]bool
	dataVersion        string

	// Focus workaround.
	everFocused []widget.Focuser
}

func (p *UI) Init(w, h int, configFile, inputFile, requestedPort string) error {
	ebiten.SetWindowSize(w, h)
	ebiten.SetWindowTitle("MIDI Converser - graphical player")
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetWindowClosingHandled(true)

	p.configFile = configFile
	p.inputFile = inputFile
	p.requestedPort = requestedPort

	// TODO: refactor to load the config override only once.
	var initialPWConfig processor.Config
	err := loadConfigOverride(p.configFile, &initialPWConfig)
	if err != nil {
		log.Printf("Failed to load config override for password: %v.", err)
	}

	fsys, err := openFS(initialPWConfig.DataPassword)
	if err != nil {
		log.Printf("Failed to open FS, so deferring for later: %v", err)
	}

	p.dataVersion = "(not loaded yet)"

	err = p.initWithFS(fsys)
	if err != nil {
		return err
	}

	p.initChannelsList()

	err = p.initUI()
	if err != nil {
		return err
	}

	return nil
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
	to.OutputPort = from.OutputPort
	to.DataPassword = from.DataPassword
}

func (p *UI) initWithFS(fsys fs.FS) error {
	if fsys == nil {
		// Need to reinit later.
		return nil
	}

	p.initDataVersion(fsys)

	err := p.initBackend(fsys)
	if err != nil {
		return err
	}

	return nil
}

func (p *UI) initBackend(fsys fs.FS) error {
	var err error
	p.config, err = loadConfig(fsys, p.configFile)
	if err != nil {
		return fmt.Errorf("failed to read config: %w", err)
	}

	err = loadConfigOverride(p.configFile, p.config)
	if err != nil {
		log.Printf("Failed to load config override: %v.", err)
	}

	err = p.initHymnsList(fsys)
	if err != nil {
		return err
	}

	p.outPort, err = player.FindBestPort(p.requestedPort, p.config.OutputPort)
	if err != nil {
		log.Printf("Could not find MIDI port: %v - continuning without; playing will fail.", err)
	}
	log.Printf("Picked output port: %v.", p.outPort)

	p.backend = player.NewBackend(&player.Options{
		FSys:     fsys,
		Config:   p.config,
		OutPort:  p.outPort,
		PlayOnly: p.inputFile,
	})

	p.mustRecreateUI = true

	go func() {
		p.loopErr = p.backend.Loop()
		p.backend.Close()
	}()

	return nil
}

func (p *UI) Shutdown() {
	if p.backend == nil {
		// Never launched.
		return
	}
	close(p.backend.Commands)
	for {
		_, ok := <-p.backend.UIStates
		if !ok {
			break
		}
	}
	p.backend = nil
}

func (p *UI) initDataVersion(fsys fs.FS) {
	v, err := fs.ReadFile(fsys, "version.txt")
	if err != nil {
		p.dataVersion = "(unknown)"
		log.Printf("Could not read version file: %v.", err)
		return
	}
	p.dataVersion = string(bytes.TrimSpace(v))
}

var splitNumbersRE = regexp.MustCompile(`\d+`)

func splitNumbers(s string) string {
	parts := splitNumbersRE.FindAllStringIndex(s, -1)
	var ret []string
	lastIndex := 0
	for _, part := range parts {
		begin, end := part[0], part[1]
		if begin > lastIndex {
			ret = append(ret, s[lastIndex:begin])
		}
		var num int64
		fmt.Sscanf(s[begin:end], "%d", &num)
		ret = append(ret, fmt.Sprintf("\000%08d\000", num))
		lastIndex = end
	}
	if len(s) > lastIndex {
			ret = append(ret, s[lastIndex:len(s)])
	}
	return strings.Join(ret, "")
}

func listHymns(fsys fs.FS, subdir string) ([]string, []string, error) {
	all, err := fs.Glob(fsys, path.Join(subdir, "*.yml"))
	if err != nil {
		return nil, nil, fmt.Errorf("glob: %w", err)
	}

	var hymns []string
	tagsMap := map[string]bool{}
	for _, name := range all {
		options, err := file.ReadOptions(fsys, name)
		if err != nil {
			log.Printf("Skipping file %v because it seems to not be a hymn: %v.", name, err)
			continue
		}
		f, err := fsys.Open(options.InputFile)
		if err != nil {
			log.Printf("Skipping file %v because input file is not available: %v.", name, err)
			continue
		}
		f.Close()
		hymns = append(hymns, name)
		for _, t := range options.Tags {
			tagsMap[t] = true
		}
	}

	slices.SortFunc(hymns, func(a, b string) int {
		aNum, bNum := splitNumbers(a), splitNumbers(b)
		return cmp.Compare(aNum, bNum)
	})

	var tags []string
	for t := range tagsMap {
		tags = append(tags, t)
	}
	slices.Sort(tags)

	return hymns, tags, nil
}

func (p *UI) hymnsSubdirs() []string {
	var ret []string
	if p.config.HymnsSubdir != "" {
		ret = append(ret, p.config.HymnsSubdir)
	}
	locs, err := locale.GetLocales()
	if err != nil {
		log.Printf("Could not detect locales - working without: %v.", err)
	}
	for _, loc := range locs {
		lang, err := language.Parse(loc)
		if err != nil {
			ret = append(ret, loc)
			continue
		}
		for lang != language.Und {
			ret = append(ret, lang.String())
			lang = lang.Parent()
		}
	}
	ret = append(ret, "en")
	return ret
}

func (p *UI) initHymnsList(fsys fs.FS) error {
	for _, hymnsSubdir := range p.hymnsSubdirs() {
		hymns, tags, err := listHymns(fsys, hymnsSubdir)
		if err != nil {
			return err
		}
		if len(hymns) != 0 {
			p.config.HymnsSubdir = hymnsSubdir
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
	}
	return fmt.Errorf("could not find any hymns")
}

func (p *UI) initChannelsList() {
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

func (p *UI) initOutPortsList() {
	if p.outPorts != nil && p.outPort == nil {
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

func (p *UI) outPortNameFunc(e any) string {
	if e == nil {
		return "none"
	}
	return p.outPorts[e.(int)].String()
}

func (p *UI) tagNameFunc(e any) string {
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

func (p *UI) initUI() error {
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

func (p *UI) recreateUI() {
	fontSize := 4.0 * p.scale
	smallFontSize := 2.0 * p.scale
	spacing := int(math.Round(3 * p.scale))
	listSliderSize := int(math.Round(8 * p.scale))
	buttonInsets := int(math.Round(p.scale))
	checkSize := int(math.Round(3 * p.scale))
	sliderMinHandleSize := int(math.Round(4 * p.scale))
	tempoSliderSize := int(math.Round(8 * p.scale))
	keySpacing := int(math.Round(p.scale))

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
		Idle:     color.White,
		Hover:    color.Gray{Y: 64},
		Pressed:  color.Black,
		Disabled: color.Gray{Y: 192},
	}
	buttonImage := &widget.ButtonImage{
		Idle:     image.NewNineSliceColor(color.Black),
		Hover:    image.NewNineSliceColor(color.Gray{Y: 192}),
		Pressed:  image.NewNineSliceColor(color.White),
		Disabled: image.NewNineSliceColor(color.Gray{Y: 64}),
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
		Unchecked: &widget.GraphicImage{Idle: newImageColor(checkSize, color.NRGBA{R: 128, G: 128, B: 128, A: 32})},
		Checked:   &widget.GraphicImage{Idle: newImageColor(checkSize, color.NRGBA{R: 128, G: 128, B: 128, A: 255})},
		Greyed:    &widget.GraphicImage{Idle: newImageColor(checkSize, color.Alpha{A: 0})},
	}
	textInputImage := &widget.TextInputImage{
		Idle:     image.NewNineSliceColor(color.Gray{Y: 192}),
		Disabled: image.NewNineSliceColor(color.Gray{Y: 224}),
	}
	textInputColor := &widget.TextInputColor{
		Idle:          color.Black,
		Disabled:      color.Black,
		Caret:         color.Gray{Y: 128},
		DisabledCaret: color.Gray{Y: 128},
	}

	// Rebuild the rootContainer.

	p.rootContainer.RemoveChildren()

	topInsets := widget.Insets{
		Left:   spacing + p.insets.Left,
		Top:    spacing + p.insets.Top,
		Right:  spacing + p.insets.Right,
		Bottom: spacing + p.insets.Bottom,
	}

	mainContainer := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(1),
			widget.GridLayoutOpts.Spacing(spacing, spacing),
			widget.GridLayoutOpts.Padding(topInsets),
			widget.GridLayoutOpts.Stretch([]bool{true}, []bool{false, false, true}),
		)),
		widget.ContainerOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.AnchorLayoutData{
				StretchHorizontal: true,
				StretchVertical:   true,
			})),
	)
	p.rootContainer.AddChild(mainContainer)

	versionLabel := widget.NewLabel(
		widget.LabelOpts.Text(fmt.Sprintf("Version: code %s, data %s", version.Version(), p.dataVersion), smallFontFace, labelColors),
		widget.LabelOpts.TextOpts(
			widget.TextOpts.Position(widget.TextPositionEnd, widget.TextPositionCenter),
		),
	)
	mainContainer.AddChild(versionLabel)

	tempoBarContainer := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(2),
			widget.GridLayoutOpts.Spacing(spacing, spacing),
			widget.GridLayoutOpts.Stretch([]bool{true, false}, []bool{true}),
		)),
	)
	mainContainer.AddChild(tempoBarContainer)

	buttonsContainer := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(1),
			widget.GridLayoutOpts.Spacing(spacing, spacing),
			widget.GridLayoutOpts.Stretch([]bool{true}, []bool{true}),
		)),
	)
	tempoBarContainer.AddChild(buttonsContainer)

	tableContainer := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(2),
			widget.GridLayoutOpts.Spacing(spacing, spacing),
			widget.GridLayoutOpts.Stretch([]bool{false, true}, []bool{false}),
		)),
	)
	buttonsContainer.AddChild(tableContainer)

	currentlyPlayingLabel := widget.NewLabel(
		widget.LabelOpts.Text("Playing: ", fontFace, labelColors),
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
		widget.LabelOpts.Text("At: ...", fontFace, labelColors),
	)
	tableContainer.AddChild(p.playbackLabel)

	p.playbackOrSkip = widget.NewFlipBook()
	tableContainer.AddChild(p.playbackOrSkip)

	p.playback = widget.NewProgressBar(
		widget.ProgressBarOpts.Images(progressTrackImage, progressImage),
		widget.ProgressBarOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.AnchorLayoutData{
				StretchHorizontal: true,
				StretchVertical:   true,
			})),
	)
	p.playbackOrSkip.SetPage(p.playback)
	p.skipVisible = false

	p.skip = widget.NewButton(
		widget.ButtonOpts.Text("(skip)", fontFace, buttonTextColor),
		widget.ButtonOpts.Image(buttonImage),
		widget.ButtonOpts.TextPadding(widget.Insets{Left: buttonInsets, Right: buttonInsets}),
		widget.ButtonOpts.ClickedHandler(p.skipClicked),
		widget.ButtonOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.AnchorLayoutData{
				StretchHorizontal: true,
				StretchVertical:   true,
			})),
	)
	p.skip.GetWidget().Disabled = true

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

	p.tempoLabel = widget.NewLabel(
		widget.LabelOpts.Text("T=...", fontFace, labelColors),
		widget.LabelOpts.TextOpts(
			widget.TextOpts.Position(widget.TextPositionEnd, widget.TextPositionCenter),
		),
	)
	versesContainer.AddChild(p.tempoLabel)

	p.tempo = widget.NewSlider(
		// Tempo value is negated so up is faster.
		widget.SliderOpts.MinMax(-125, -80),
		widget.SliderOpts.Images(sliderTrackImage, sliderButtonImage),
		widget.SliderOpts.MinHandleSize(tempoSliderSize),
		widget.SliderOpts.ChangedHandler(p.tempoChanged),
		widget.SliderOpts.PageSizeFunc(func() int {
			return 1
		}),
		widget.SliderOpts.Direction(widget.DirectionVertical),
	)
	p.prevTempo = -1 // Force refresh.
	tempoBarContainer.AddChild(p.tempo)

	playContainer := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(3),
			widget.GridLayoutOpts.Spacing(spacing, spacing),
			widget.GridLayoutOpts.Stretch([]bool{true, false, false}, []bool{false}),
		)),
	)
	buttonsContainer.AddChild(playContainer)

	p.stop = widget.NewButton(
		widget.ButtonOpts.Text("Stop", fontFace, buttonTextColor),
		widget.ButtonOpts.Image(buttonImage),
		widget.ButtonOpts.TextPadding(widget.NewInsetsSimple(buttonInsets)),
		widget.ButtonOpts.ClickedHandler(p.stopClicked),
	)
	playContainer.AddChild(p.stop)

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

	settings := widget.NewButton(
		widget.ButtonOpts.Text("Settings...", fontFace, buttonTextColor),
		widget.ButtonOpts.Image(buttonImage),
		widget.ButtonOpts.TextPadding(widget.NewInsetsSimple(buttonInsets)),
		widget.ButtonOpts.ClickedHandler(p.settingsClicked),
	)
	buttonsContainer.AddChild(settings)

	p.prompt = widget.NewButton(
		widget.ButtonOpts.Text("(prompt)", fontFace, buttonTextColor),
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
		widget.ListOpts.SliderOpts(
			widget.SliderOpts.Images(sliderTrackImage, sliderButtonImage),
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
		widget.ListOpts.SliderOpts(
			widget.SliderOpts.Images(sliderTrackImage, sliderButtonImage),
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
		widget.ListOpts.SliderOpts(
			widget.SliderOpts.Images(sliderTrackImage, sliderButtonImage),
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
			widget.ListOpts.SliderOpts(
				widget.SliderOpts.Images(sliderTrackImage, sliderButtonImage),
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
			widget.ListOpts.SliderOpts(
				widget.SliderOpts.Images(sliderTrackImage, sliderButtonImage),
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
			widget.ListOpts.SliderOpts(
				widget.SliderOpts.Images(sliderTrackImage, sliderButtonImage),
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
		widget.SliderOpts.MinHandleSize(sliderMinHandleSize),
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
		widget.SliderOpts.MinHandleSize(sliderMinHandleSize),
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
		widget.SliderOpts.MinHandleSize(sliderMinHandleSize),
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

	// Rebuild the password window.

	passwordWindowContainer := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(image.NewNineSliceColor(color.Gray{Y: 224})),
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(1),
			widget.GridLayoutOpts.Spacing(spacing, spacing),
			widget.GridLayoutOpts.Padding(widget.NewInsetsSimple(spacing)),
			widget.GridLayoutOpts.Stretch([]bool{true}, []bool{false, true, false}),
		)),
	)

	passwordLabel := widget.NewLabel(
		widget.LabelOpts.Text("Enter password: ", fontFace, labelColors),
	)
	passwordWindowContainer.AddChild(passwordLabel)
	p.password = widget.NewTextInput(
		widget.TextInputOpts.Image(textInputImage),
		widget.TextInputOpts.Face(fontFace),
		widget.TextInputOpts.Color(textInputColor),
		widget.TextInputOpts.Padding(widget.Insets{Left: buttonInsets, Right: buttonInsets}),
		widget.TextInputOpts.CaretOpts(
			widget.CaretOpts.Size(fontFace, 2),
		),
		widget.TextInputOpts.Secure(true),
		widget.TextInputOpts.Placeholder("Password"),
	)
	passwordWindowContainer.AddChild(p.password)

	p.vKeys = nil
	for _, row := range vKeys {
		rowContainer := widget.NewContainer(
			widget.ContainerOpts.BackgroundImage(image.NewNineSliceColor(color.Gray{Y: 224})),
			widget.ContainerOpts.Layout(widget.NewGridLayout(
				widget.GridLayoutOpts.Columns(len(row)),
				widget.GridLayoutOpts.Spacing(keySpacing, keySpacing),
				widget.GridLayoutOpts.Padding(widget.NewInsetsSimple(keySpacing)),
				widget.GridLayoutOpts.Stretch([]bool{true, true, true, true, true, true, true, true, true, true}, []bool{true}),
			)),
		)
		passwordWindowContainer.AddChild(rowContainer)
		var vRow []*widget.Button
		for range row {
			btn := widget.NewButton(
				widget.ButtonOpts.Text("", fontFace, buttonTextColor),
				widget.ButtonOpts.Image(buttonImage),
				widget.ButtonOpts.TextPadding(widget.NewInsetsSimple(buttonInsets)),
				widget.ButtonOpts.ClickedHandler(p.passwordKeyPressed),
			)
			rowContainer.AddChild(btn)
			vRow = append(vRow, btn)
		}
		p.vKeys = append(p.vKeys, vRow)
	}
	p.vKeysUpdate()

	applyPassword := widget.NewButton(
		widget.ButtonOpts.Text("OK", fontFace, buttonTextColor),
		widget.ButtonOpts.Image(buttonImage),
		widget.ButtonOpts.TextPadding(widget.NewInsetsSimple(buttonInsets)),
		widget.ButtonOpts.ClickedHandler(p.applyPasswordClicked),
	)
	passwordWindowContainer.AddChild(applyPassword)

	passwordTitleContainer := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(image.NewNineSliceColor(color.Black)),
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(1),
			widget.GridLayoutOpts.Spacing(spacing, spacing),
			widget.GridLayoutOpts.Stretch([]bool{true}, []bool{true}),
		)),
	)
	passwordTitle := widget.NewText(
		widget.TextOpts.Text("Password", fontFace, color.White),
		widget.TextOpts.Insets(widget.Insets{Left: buttonInsets, Right: buttonInsets}),
		widget.TextOpts.Position(widget.TextPositionStart, widget.TextPositionCenter),
	)
	passwordTitleContainer.AddChild(passwordTitle)

	if p.passwordWindowOpen {
		p.passwordWindow.Close()
	}

	p.passwordWindow = widget.NewWindow(
		widget.WindowOpts.Contents(passwordWindowContainer),
		widget.WindowOpts.TitleBar(passwordTitleContainer, titleBarHeight),
		widget.WindowOpts.Modal(),
		widget.WindowOpts.CloseMode(widget.NONE),
	)

	if p.passwordWindowOpen {
		p.openPasswordWindow(nil)
	}
}

func (p *UI) setFocus(widget widget.Focuser) {
	// This workaround should not be necessary, as p.ui.ClearFocus() should already do it...
	// TODO: Investigate.
	known := false
	for _, w := range p.everFocused {
		if w == widget {
			known = true
			continue
		}
		w.Focus(false)
	}
	p.ui.ClearFocus()
	if widget != nil {
		widget.Focus(true)
		if !known {
			p.everFocused = append(p.everFocused, widget)
		}
	}
}

func (p *UI) stopClicked(args *widget.ButtonClickedEventArgs) {
	if p.backend == nil {
		return
	}
	p.setFocus(nil)
	p.backend.Commands <- player.Command{
		Exit: true,
	}
}

func (p *UI) promptClicked(args *widget.ButtonClickedEventArgs) {
	if p.backend == nil {
		return
	}
	// Reset focus to stop.
	p.setFocus(p.stop)
	p.backend.Commands <- player.Command{
		Answer: true,
	}
}

func (p *UI) skipClicked(args *widget.ButtonClickedEventArgs) {
	if p.backend == nil {
		return
	}
	// Reset focus to prompt.
	p.setFocus(p.prompt)
	p.backend.Commands <- player.Command{
		Skip: true,
	}
}

func (p *UI) tempoChanged(args *widget.SliderChangedEventArgs) {
	if p.backend == nil {
		return
	}
	p.backend.Commands <- player.Command{
		// Tempo value is negated so up is faster.
		Tempo: -float64(args.Current) / 100.0,
	}
}

func (p *UI) fewerVersesClicked(args *widget.ButtonClickedEventArgs) {
	if p.backend == nil {
		return
	}
	p.backend.Commands <- player.Command{
		NumVerses: p.uiState.NumVerses - 1,
	}
}

func (p *UI) moreVersesClicked(args *widget.ButtonClickedEventArgs) {
	if p.backend == nil {
		return
	}
	p.backend.Commands <- player.Command{
		NumVerses: p.uiState.NumVerses + 1,
	}
}

func (p *UI) positionWindow(win *widget.Window, f float64) {
	w := p.width - 32
	_, tH := win.TitleBar.PreferredSize()
	_, cH := win.Contents.PreferredSize()
	h := tH + cH
	if h > p.height-32 {
		h = p.height - 32
	}
	x := (p.width - w) / 2
	y := (int(float64(p.height)*f) - h) / 2
	if x < 16 {
		x = 16
	}
	if y < 16 {
		y = 16
	}
	x += p.insets.Left
	y += p.insets.Top
	r := go_image.Rect(x, y, x+w, y+h)
	win.SetLocation(r)
}

func (p *UI) selectHymnClicked(args *widget.ButtonClickedEventArgs) {
	if p.backend == nil {
		return
	}
	p.positionWindow(p.hymnsWindow, 1.0)
	p.ui.AddWindow(p.hymnsWindow)
	p.hymnsWindowOpen = true
}

func (p *UI) playHymnClicked(args *widget.ButtonClickedEventArgs) {
	p.setFocus(p.stop)
	p.hymnsWindow.Close()
	p.hymnsWindowOpen = false
	if p.backend == nil {
		return
	}
	e, ok := p.hymnList.SelectedEntry().(string)
	if !ok {
		log.Printf("No hymn selected.")
		return
	}
	p.backend.Commands <- player.Command{
		PlayOne: e,
	}
}

func (p *UI) hymnsCloseClicked(args *widget.ButtonClickedEventArgs) {
	p.setFocus(nil)
	p.hymnsWindow.Close()
	p.hymnsWindowOpen = false
}

func (p *UI) selectPreludeClicked(args *widget.ButtonClickedEventArgs) {
	if p.backend == nil {
		return
	}
	p.positionWindow(p.preludeWindow, 1.0)
	p.ui.AddWindow(p.preludeWindow)
	p.preludeWindowOpen = true
}

func (p *UI) playPreludeClicked(args *widget.ButtonClickedEventArgs) {
	p.setFocus(p.stop)
	p.preludeWindow.Close()
	p.preludeWindowOpen = false
	if p.backend == nil {
		return
	}
	p.backend.Commands <- player.Command{
		PlayPrelude: true,
	}
}

func (p *UI) preludeCloseClicked(args *widget.ButtonClickedEventArgs) {
	p.setFocus(nil)
	p.preludeWindow.Close()
	p.preludeWindowOpen = false
}

func (p *UI) avoidTagClicked(args *widget.ButtonClickedEventArgs) {
	if p.backend == nil {
		return
	}
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

func (p *UI) ignoreTagClicked(args *widget.ButtonClickedEventArgs) {
	if p.backend == nil {
		return
	}
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

func (p *UI) requestTagClicked(args *widget.ButtonClickedEventArgs) {
	if p.backend == nil {
		return
	}
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

func (p *UI) settingsClicked(args *widget.ButtonClickedEventArgs) {
	if p.backend == nil {
		return
	}
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

	p.positionWindow(p.settingsWindow, 1.0)
	p.ui.AddWindow(p.settingsWindow)
	p.settingsWindowOpen = true
}

func (p *UI) applySettingsClicked(args *widget.ButtonClickedEventArgs) {
	p.setFocus(nil)
	p.settingsWindow.Close()
	p.settingsWindowOpen = false
	if p.backend == nil {
		return
	}
	nextPort := p.settingsOutPort.SelectedEntry()
	if nextPort != nil {
		port := p.outPorts[nextPort.(int)]
		if p.outPort == nil || port.Number() != p.outPort.Number() {
			p.outPort = port
			p.backend.Commands <- player.Command{
				OutPort: port,
			}
		}
		p.config.OutputPort = port.String()
	}
	p.config.Channel = p.settingsChannel.SelectedEntry().(int)
	p.config.MelodyChannel = p.settingsMelodyChannel.SelectedEntry().(int)
	p.config.BassChannel = p.settingsBassChannel.SelectedEntry().(int)
	p.config.HoldRedundantNotes = p.settingsHoldRedundantNotes.State() == widget.WidgetChecked
	p.config.BPMFactor = float64(p.settingsTempo.Current) * 0.01
	p.config.PreludePlayerRepeat = p.settingsPreludePlayerRepeat.Current
	p.config.PreludePlayerSleepSec = float64(p.settingsPreludePlayerSleep.Current) * 0.1
	p.config.FermatasInPrelude = p.settingsFermatasInPrelude.State() == widget.WidgetChecked
	err := saveConfigOverride(p.configFile, p.config)
	if err != nil {
		log.Printf("Could not save config override with new config: %v.", err)
	}
	p.backend.Commands <- player.Command{
		Config: p.config,
	}
}

func (p *UI) settingsCloseClicked(args *widget.ButtonClickedEventArgs) {
	p.setFocus(nil)
	p.settingsWindow.Close()
	p.settingsWindowOpen = false
}

func (p *UI) openPasswordWindow(args *widget.ButtonClickedEventArgs) {
	if p.backend != nil {
		return
	}

	p.password.SetText("")

	p.positionWindow(p.passwordWindow, 0.5)
	p.ui.AddWindow(p.passwordWindow)

	p.setFocus(p.password)

	p.passwordWindowOpen = true
}

func (p *UI) passwordKeyPressed(args *widget.ButtonClickedEventArgs) {
	var action vKey
	for i, row := range p.vKeys {
		for j, btn := range row {
			if args.Button == btn {
				action = vKeys[i][j]
			}
		}
	}
	prevPW := p.password.GetText()
	prevMode := p.vKeyMode
	pw, mode := action.modeAt(prevMode).applyTo(prevPW, prevMode)
	if pw != prevPW {
		p.password.SetText(pw)
	}
	if prevMode != mode {
		p.vKeyMode = mode
		p.vKeysUpdate()
	}
}

func (p *UI) vKeysUpdate() {
	for i, row := range p.vKeys {
		for j, btn := range row {
			btn.Text().Label = vKeys[i][j].modeAt(p.vKeyMode).display
		}
	}
}

func (p *UI) applyPasswordClicked(args *widget.ButtonClickedEventArgs) {
	p.setFocus(nil)
	p.passwordWindow.Close()
	p.passwordWindowOpen = false

	if p.backend != nil {
		return
	}
	pw := p.password.GetText()
	fsys, err := openFS(pw)
	if err != nil {
		log.Printf("Could not open FS with password: %v.", err)
		return
	}
	if fsys == nil {
		log.Printf("Got no FS with password: %v.", err)
		return
	}
	err = p.initWithFS(fsys)
	if err != nil {
		log.Printf("Could not initialize with password: %v.", err)
		return
	}
	if p.backend == nil {
		log.Printf("Got no backend with password: %v.", err)
		return
	}
	p.config.DataPassword = pw
	err = saveConfigOverride(p.configFile, p.config)
	if err != nil {
		log.Printf("Could not save config override with password: %v.", err)
	}
}

// updateUI updates all widgets to current playback state.
func (p *UI) updateWidgets() {
	insets := safeAreaMargins()
	p.width = p.rawWidth - insets.Left - insets.Right
	p.height = p.rawHeight - insets.Top - insets.Bottom

	scale := math.Min(
		float64(p.width)/80,
		float64(p.height)/120,
	)
	if math.Abs(scale-p.scale) > 0.001 || p.mustRecreateUI || insets != p.insets {
		p.scale = scale
		p.insets = insets
		p.mustRecreateUI = false
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
		p.playbackLabel.Label = "At: "
		p.playbackLabel.GetWidget().Disabled = false
		p.playback.Min = 0
		p.playback.Max = 1000000
		p.playback.SetCurrent(int(math.Round(1000000 * p.uiState.ActualPlaybackFraction())))
		p.playback.GetWidget().Disabled = false
		if p.stop.GetWidget().Disabled {
			p.setFocus(p.stop)
		}
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
		// Tempo value is negated so up is faster.
		p.tempo.Current = -int(math.Round(100.0 * p.uiState.Tempo))
		p.prevTempo = p.uiState.Tempo
	}
	// Tempo value is negated so up is faster.
	p.tempoLabel.Label = fmt.Sprintf("T=%d%%", -p.tempo.Current)

	if p.uiState.NumVerses > 0 {
		postludeSuffix := ""
		if p.uiState.HavePostlude {
			postludeSuffix += "+P"
		}
		if p.uiState.UnrolledNumVerses != 0 {
			postludeSuffix += fmt.Sprintf("=%d", p.uiState.UnrolledNumVerses)
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
			p.setFocus(p.prompt)
		}
		p.prompt.Text().Label = p.uiState.Prompt
		p.prompt.GetWidget().Visibility = widget.Visibility_Show
		p.prompt.GetWidget().Disabled = false
	} else {
		p.prompt.GetWidget().Visibility = widget.Visibility_Hide_Blocking
		p.prompt.GetWidget().Disabled = true
	}

	if p.uiState.SkipPrompt != "" {
		if !p.skipVisible {
			p.playbackOrSkip.SetPage(p.skip)
			p.skipVisible = true
		}
		p.skip.Text().Label = p.uiState.SkipPrompt
		p.skip.GetWidget().Disabled = false
	} else {
		if p.skipVisible {
			p.playbackOrSkip.SetPage(p.playback)
			p.skipVisible = false
		}
		p.skip.GetWidget().Disabled = true
	}

	if !reflect.DeepEqual(p.uiState.PreludeTags, p.prevPreludeTags) {
		p.prevPreludeTags = p.uiState.PreludeTags
		selected, ok := p.preludeTagList.SelectedEntry().(string)
		p.preludeTagList.SetEntries(p.tagsAny)
		if ok {
			p.preludeTagList.SetSelectedEntry(selected)
		}
	}

	if p.backend == nil && !p.passwordWindowOpen {
		p.openPasswordWindow(nil)
	}
}

func (p *UI) Update() error {
	defer p.ui.Update()

	if ebiten.IsWindowBeingClosed() {
		if p.backend == nil {
			return player.QuitError
		} else {
			p.backend.Commands <- player.Command{
				Quit: true,
			}
		}
	}

	if p.backend != nil {
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
	}

	p.updateWidgets()

	if p.outPort == nil && !p.settingsWindowOpen {
		p.settingsClicked(nil)
	}

	wakelockSet(p.uiState.PlayOne != "" || p.uiState.PlayPrelude)

	return nil
}

func (p *UI) Draw(screen *ebiten.Image) {
	p.ui.Draw(screen)
}

func (p *UI) Layout(outsideWidth int, outsideHeight int) (int, int) {
	p.rawWidth = outsideWidth
	p.rawHeight = outsideHeight
	return p.rawWidth, p.rawHeight
}
