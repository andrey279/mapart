//go:build !windows && gui

package main

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// FyneGUI follows the layout and workflow of the native Windows MapArt Studio.
type FyneGUI struct {
	app                  fyne.App
	window               fyne.Window
	opts                 RenderOptions
	buildOpts            BuildOptions
	result               *PreviewResult
	resultOpts           RenderOptions
	resultSource         string
	structure            *Structure
	qrOptions            QRCodeOptions
	generatedSource      image.Image
	busy, dirty, loading bool
	closed               atomic.Bool

	path                                           *widget.Entry
	preset                                         *widget.Select
	presetInfo, status, dimensions, materials      *widget.Label
	settings, previewTabs                          *container.AppTabs
	progress                                       *widget.ProgressBarInfinite
	convertButton, pngButton, nbtButton, zipButton *widget.Button
	editorButton                                   *widget.Button
	zoom                                           *widget.Check
	panes                                          []*studioImagePane
	sourcePane, resultPane, mapPane, maskPane      *studioImagePane
	controls                                       []fyne.Disableable
	readFields, readBuildFields                    []func() error
	writeFields                                    []func()
	syncBlockControls                              func()
	blockPicker                                    *studioBlockPicker
}

type studioImagePane struct {
	image   *canvas.Image
	scroll  *container.Scroll
	content fyne.CanvasObject
}

type studioTheme struct{ fyne.Theme }

func (t studioTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	if name == theme.ColorNamePrimary {
		return color.NRGBA{R: 27, G: 79, B: 120, A: 255}
	}
	return t.Theme.Color(name, variant)
}

func RunFyneGUI(opts RenderOptions, path string, buildOpts BuildOptions) error {
	g := &FyneGUI{
		app:  app.NewWithID("com.mapartscript.app"),
		opts: opts, buildOpts: buildOpts, loading: true,
	}
	g.app.Settings().SetTheme(studioTheme{theme.LightTheme()})
	g.window = g.app.NewWindow("MapArt Studio")
	g.window.Resize(fyne.NewSize(1320, 850))
	g.window.SetOnClosed(func() { g.closed.Store(true) })
	g.setupUI()
	g.path.SetText(path)
	g.writeOptions(opts)
	g.matchPreset(opts)
	g.window.SetOnDropped(func(_ fyne.Position, uris []fyne.URI) {
		if !g.busy && len(uris) > 0 && uris[0].Scheme() == "file" {
			g.path.SetText(uris[0].Path())
			g.convertImage()
		}
	})
	g.window.Show()
	if path != "" {
		g.convertImage()
	}
	g.app.Run()
	return nil
}

func wrapped(text string) *widget.Label {
	label := widget.NewLabel(text)
	label.Wrapping = fyne.TextWrapWord
	return label
}

func (g *FyneGUI) track(control fyne.Disableable) {
	g.controls = append(g.controls, control)
}

func (g *FyneGUI) action(text string, icon fyne.Resource, callback func()) *widget.Button {
	button := widget.NewButtonWithIcon(text, icon, callback)
	g.track(button)
	return button
}

func (g *FyneGUI) setupUI() {
	g.status = wrapped("Выберите изображение. Все операции выполняются локально.")
	g.dimensions = widget.NewLabel("Превью появится после преобразования")
	g.progress = widget.NewProgressBarInfinite()
	g.progress.Stop()
	g.progress.Hide()

	title := canvas.NewText("MAPART", color.NRGBA{R: 27, G: 79, B: 120, A: 255})
	title.TextSize = 26
	title.TextStyle.Bold = true
	journey := container.NewHBox(widget.NewLabel("Картинка"), widget.NewIcon(theme.NavigateNextIcon()),
		widget.NewLabel("карта"), widget.NewIcon(theme.NavigateNextIcon()), widget.NewLabel("постройка"))
	header := container.NewHBox(title, journey,
		layout.NewSpacer(), g.action("QR-код…", theme.GridIcon(), g.qrGenerator), widget.NewButton("О программе", g.showAbout))
	g.path = widget.NewEntry()
	g.path.SetPlaceHolder("Выберите картинку или перетащите её в окно")
	g.path.OnChanged = func(string) { g.changed() }
	g.path.OnSubmitted = func(string) { g.convertImage() }
	g.track(g.path)
	sourceRow := container.NewBorder(nil, nil, nil,
		container.NewHBox(g.action("Редактор исходника…", theme.DocumentIcon(), g.editSource),
			g.action("Открыть картинку…", theme.FolderOpenIcon(), g.openImageDialog)), g.path)

	g.presetInfo = wrapped("")
	g.preset = widget.NewSelect(PresetNames(), func(name string) {
		g.presetInfo.SetText(PresetDescription(name))
	})
	g.preset.SetSelected("balanced")
	g.track(g.preset)
	presetRow := container.NewBorder(nil, nil, widget.NewLabel("Пресет"),
		g.action("Применить", nil, g.applyPreset), g.preset)

	g.settings = container.NewAppTabs(g.optionPages()...)
	profiles := container.NewGridWithColumns(2,
		g.action("Загрузить JSON…", theme.FolderOpenIcon(), g.loadProfile),
		g.action("Сохранить JSON…", theme.DocumentSaveIcon(), g.saveProfile))
	g.convertButton = widget.NewButtonWithIcon("Преобразовать", theme.MediaPlayIcon(), g.convertImage)
	g.convertButton.Importance = widget.HighImportance
	left := container.NewBorder(container.NewVBox(presetRow, g.presetInfo),
		container.NewVBox(profiles, g.convertButton), nil, nil, g.settings)

	g.sourcePane = g.imagePane(false)
	g.resultPane = g.imagePane(true)
	g.mapPane = g.imagePane(true)
	g.maskPane = g.imagePane(true)
	comparison := container.NewHSplit(
		widget.NewCard("Исходник", "", g.sourcePane.content),
		widget.NewCard("Мап арт", "", g.resultPane.content))
	comparison.Offset = 0.5
	g.materials = wrapped("После преобразования здесь появятся размеры схемы, материалы и инструкция по размещению.")
	copyButton := widget.NewButtonWithIcon("Копировать", theme.ContentCopyIcon(), func() {
		g.window.Clipboard().SetContent(g.materials.Text)
	})
	materialTab := container.NewBorder(container.NewHBox(layout.NewSpacer(), copyButton),
		nil, nil, nil, container.NewVScroll(g.materials))
	g.previewTabs = container.NewAppTabs(
		container.NewTabItem("Сравнение", comparison),
		container.NewTabItem("Карта в игре", g.mapPane.content),
		container.NewTabItem("Маска фона", g.maskPane.content),
		container.NewTabItem("Материалы и размещение", materialTab))
	g.zoom = widget.NewCheck("Размер 1:1", func(bool) { g.setZoom() })
	previewHeader := container.NewBorder(nil, nil, nil, g.zoom, g.dimensions)
	g.pngButton = widget.NewButton("Сохранить PNG…", g.exportPNG)
	g.nbtButton = widget.NewButton("Схема .nbt…", func() { g.exportNBT(false) })
	g.zipButton = widget.NewButton("ZIP по картам…", func() { g.exportNBT(true) })
	g.editorButton = widget.NewButton("Редактор карты…", g.editMap)
	exports := container.NewHBox(g.editorButton, g.pngButton, g.nbtButton, g.zipButton)
	right := container.NewBorder(previewHeader, exports, nil, nil, g.previewTabs)
	split := container.NewHSplit(left, right)
	split.Offset = 0.38
	g.window.SetContent(container.NewPadded(container.NewBorder(
		container.NewVBox(header, sourceRow), container.NewVBox(g.progress, g.status),
		nil, nil, split)))
	g.updateButtons()
}

func (g *FyneGUI) imagePane(pixels bool) *studioImagePane {
	img := canvas.NewImageFromImage(nil)
	img.FillMode = canvas.ImageFillContain
	img.SetMinSize(fyne.NewSize(100, 100))
	if pixels {
		img.ScaleMode = canvas.ImageScalePixels
	}
	background := canvas.NewRectangle(color.NRGBA{R: 235, G: 238, B: 242, A: 255})
	scroll := container.NewScroll(container.New(studioPreviewLayout{image: img, pixels: pixels}, img))
	scroll.Direction = container.ScrollNone
	pane := &studioImagePane{img, scroll, container.NewStack(background, scroll)}
	g.panes = append(g.panes, pane)
	return pane
}

func (g *FyneGUI) showImage(pane *studioImagePane, img image.Image) {
	pane.image.Image = img
	pane.image.Refresh()
	g.setZoom()
}

func (g *FyneGUI) setZoom() {
	zoom := g.zoom != nil && g.zoom.Checked
	for _, pane := range g.panes {
		pane.image.SetMinSize(fyne.NewSize(100, 100))
		pane.image.FillMode = canvas.ImageFillContain
		pane.scroll.Direction = container.ScrollNone
		if zoom && pane.image.Image != nil {
			pane.image.FillMode = canvas.ImageFillOriginal
			pane.scroll.Direction = container.ScrollBoth
		}
		pane.image.Refresh()
		pane.scroll.Refresh()
	}
}

func (g *FyneGUI) number(label string, get func() float64, set func(float64), low, high float64, decimals int, build bool) fyne.CanvasObject {
	entry := widget.NewEntry()
	entry.Validator = func(text string) error {
		value, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
		if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value < low || value > high {
			return fmt.Errorf("%s: требуется число от %g до %g", label, low, high)
		}
		if decimals == 0 && value != math.Trunc(value) {
			return fmt.Errorf("%s: требуется целое число", label)
		}
		return nil
	}
	read := func() error {
		if err := entry.Validate(); err != nil {
			return err
		}
		value, _ := strconv.ParseFloat(strings.TrimSpace(entry.Text), 64)
		set(value)
		return nil
	}
	if build {
		entry.SetText(strconv.FormatFloat(get(), 'f', decimals, 64))
		g.readBuildFields = append(g.readBuildFields, read)
		entry.OnChanged = func(string) {
			if !g.loading && g.result != nil {
				g.status.SetText("Настройки постройки применятся при следующем экспорте NBT.")
			}
		}
	} else {
		g.readFields = append(g.readFields, read)
		g.writeFields = append(g.writeFields, func() {
			entry.SetText(strconv.FormatFloat(get(), 'f', decimals, 64))
		})
		entry.OnChanged = func(string) { g.changed() }
	}
	g.track(entry)
	step := 1.0
	if decimals > 0 {
		step = .05
	}
	adjust := func(delta float64) {
		if err := entry.Validate(); err != nil {
			g.showError(err)
			return
		}
		value, _ := strconv.ParseFloat(strings.TrimSpace(entry.Text), 64)
		value = math.Max(low, math.Min(high, value+delta))
		entry.SetText(strconv.FormatFloat(value, 'f', decimals, 64))
	}
	buttons := container.NewHBox(
		g.action("", theme.ContentRemoveIcon(), func() { adjust(-step) }),
		g.action("", theme.ContentAddIcon(), func() { adjust(step) }))
	return widget.NewForm(widget.NewFormItem(label, container.NewBorder(nil, nil, nil, buttons, entry)))
}

func (g *FyneGUI) choice(label string, get func() string, set func(string), values []string) fyne.CanvasObject {
	field := widget.NewSelect(values, func(string) { g.changed() })
	g.track(field)
	g.readFields = append(g.readFields, func() error { set(field.Selected); return nil })
	g.writeFields = append(g.writeFields, func() { field.SetSelected(get()) })
	return widget.NewForm(widget.NewFormItem(label, field))
}

func (g *FyneGUI) check(label string, get func() bool, set func(bool), build bool) fyne.CanvasObject {
	field := widget.NewCheck(label, nil)
	g.track(field)
	if build {
		field.SetChecked(get())
		g.readBuildFields = append(g.readBuildFields, func() error { set(field.Checked); return nil })
	} else {
		g.readFields = append(g.readFields, func() error { set(field.Checked); return nil })
		g.writeFields = append(g.writeFields, func() { field.SetChecked(get()) })
		field.OnChanged = func(value bool) { set(value); g.changed() }
	}
	return field
}

func (g *FyneGUI) text(label string, get func() string, set func(string), file bool) fyne.CanvasObject {
	field := widget.NewEntry()
	g.track(field)
	field.OnChanged = func(string) { g.changed() }
	g.readFields = append(g.readFields, func() error { set(strings.TrimSpace(field.Text)); return nil })
	g.writeFields = append(g.writeFields, func() { field.SetText(get()) })
	var content fyne.CanvasObject = field
	if file {
		browse := g.action("…", nil, func() {
			g.openFile(label, nil, func(path string) { field.SetText(path) })
		})
		content = container.NewBorder(nil, nil, nil, browse, field)
	}
	return container.NewVBox(wrapped(label), content)
}

func (g *FyneGUI) optionPages() []*container.TabItem {
	o := &g.opts
	n := func(label string, get func() float64, set func(float64), low, high float64, decimals int) fyne.CanvasObject {
		return g.number(label, get, set, low, high, decimals, false)
	}
	check := func(label string, get func() bool, set func(bool)) fyne.CanvasObject {
		return g.check(label, get, set, false)
	}
	page := func(title string, objects ...fyne.CanvasObject) *container.TabItem {
		return container.NewTabItem(title, container.NewVScroll(container.NewPadded(container.NewVBox(objects...))))
	}
	return []*container.TabItem{
		g.blockPage(),
		page("Размер",
			n("Карт по горизонтали", func() float64 { return float64(o.MapsX) }, func(v float64) { o.MapsX = int(v) }, 1, 256, 0),
			n("Карт по вертикали", func() float64 { return float64(o.MapsY) }, func(v float64) { o.MapsY = int(v) }, 1, 256, 0),
			wrapped("Одна карта — 128×128 блоков. 5×3 = 640×384. Всего не более 256 карт."),
			g.choice("Вписывание", func() string { return o.Fit }, func(v string) { o.Fit = v }, []string{"stretch", "contain", "cover"}),
			wrapped("stretch — растянуть; contain — поля; cover — обрезка по центру."),
			g.choice("Масштабирование", func() string { return o.Resample }, func(v string) { o.Resample = v }, []string{"bicubic", "bilinear", "nearest"}),
			g.text("Обрезка: x,y,ширина,высота (можно пусто)", func() string { return o.Crop }, func(v string) { o.Crop = v }, false),
			g.text("Цвет полей и прозрачности #RRGGBB", func() string { return o.Matte }, func(v string) { o.Matte = v }, false)),
		page("Дизеринг",
			g.choice("Режим", func() string { return o.Dither }, func(v string) { o.Dither = v }, []string{"fs", "ordered", "none"}),
			n("Сила", func() float64 { return o.DitherStrength }, func(v float64) { o.DitherStrength = v }, 0, 1, 2),
			n("Порог Delta E", func() float64 { return o.Threshold }, func(v float64) { o.Threshold = v }, 0, 1000000, 2),
			g.choice("Подбор цвета", func() string { return o.ColorMetric }, func(v string) { o.ColorMetric = v }, []string{"lab", "rgb"}),
			check("Приглушать цветные точки в дымке", func() bool { return o.CleanHaze }, func(v bool) { o.CleanHaze = v }),
			wrapped("fs — рассеивание ошибки; ordered — регулярная сетка; none — без дизеринга.\nПодавление точек в дымке действует в fs и может ослабить пастель."),
			check("Только нейтральные цвета", func() bool { return o.Monochrome }, func(v bool) { o.Monochrome = v })),
		page("Цвет",
			check("Полная палитра блоков (Java 26.3)", func() bool { return o.AllBlocks }, func(v bool) { o.AllBlocks = v }),
			check("Добавить терракоту (96 оттенков, PNG и NBT)", func() bool { return o.Terracotta }, func(v bool) { o.Terracotta = v }),
			check("Добавить природные блоки (+18 оттенков, PNG и NBT)", func() bool { return o.NaturalBlocks }, func(v bool) { o.NaturalBlocks = v }),
			n("Яркость", func() float64 { return o.Brightness }, func(v float64) { o.Brightness = v }, -100, 100, 1),
			n("Контраст", func() float64 { return o.Contrast }, func(v float64) { o.Contrast = v }, .1, 3, 2),
			n("Насыщенность", func() float64 { return o.Saturation }, func(v float64) { o.Saturation = v }, 0, 3, 2),
			n("Гамма", func() float64 { return o.Gamma }, func(v float64) { o.Gamma = v }, .2, 3, 2),
			n("Температура", func() float64 { return o.Temperature }, func(v float64) { o.Temperature = v }, -100, 100, 1),
			n("Резкость", func() float64 { return o.Sharpen }, func(v float64) { o.Sharpen = v }, 0, 2, 2),
			n("Подавление шума", func() float64 { return o.Denoise }, func(v float64) { o.Denoise = v }, 0, 1, 2),
			check("Автоматический контраст", func() bool { return o.AutoContrast }, func(v bool) { o.AutoContrast = v })),
		page("Маски",
			g.choice("Ровный фон", func() string { return o.Background }, func(v string) { o.Background = v }, []string{"auto", "off", "color"}),
			g.text("Цвет фона #RRGGBB (для режима color)", func() string { return o.BackgroundColor }, func(v string) { o.BackgroundColor = v }, false),
			n("Допуск фона", func() float64 { return o.BackgroundTolerance }, func(v float64) { o.BackgroundTolerance = v }, 0, 150, 1),
			check("Убирать дизеринг со светлого неба", func() bool { return o.SkyCleanup }, func(v bool) { o.SkyCleanup = v }),
			g.text("Маска без дизеринга (белым выделить фон)", func() string { return o.NoDitherMask }, func(v string) { o.NoDitherMask = v }, true),
			g.text("Маска деталей (белым вернуть дизеринг)", func() string { return o.DetailMask }, func(v string) { o.DetailMask = v }, true),
			g.text("Своя RGB-палитра JSON (только PNG)", func() string { return o.PaletteFile }, func(v string) { o.PaletteFile = v }, true)),
		page("Постройка",
			g.modeChoice(),
			wrapped("2D — все блоки на одной высоте, один оттенок на блок.\n3D — лестница, три оттенка за счёт перепадов высоты. Северный служебный ряд добавляется автоматически."),
			g.check("Опоры из булыжника под перепадами", func() bool { return g.buildOpts.Supports }, func(v bool) { g.buildOpts.Supports = v }, true),
			g.number("Максимальная высота", func() float64 { return float64(g.buildOpts.MaxHeight) }, func(v float64) { g.buildOpts.MaxHeight = int(v) }, 2, 4096, 0, true),
			g.number("DataVersion NBT", func() float64 { return float64(g.buildOpts.DataVersion) }, func(v float64) { g.buildOpts.DataVersion = int(v) }, 1519, math.MaxInt32, 0, true),
			wrapped("5023 = Java 26.3 (полный каталог). 3465 = Java 1.20.1 для старых материалов. Произвольная RGB-палитра не задаёт блоки для NBT.\n\nZIP по картам содержит независимые постройки 128×128. Обязательные опоры под сыпучими и тонкими блоками сохраняются даже при отключении обычных опор.")),
	}
}

func (g *FyneGUI) readOptions() (RenderOptions, error) {
	for _, read := range g.readFields {
		if err := read(); err != nil {
			return g.opts, err
		}
	}
	return g.opts, ValidateOptions(g.opts)
}

func (g *FyneGUI) readBuild() (BuildOptions, error) {
	for _, read := range g.readBuildFields {
		if err := read(); err != nil {
			return g.buildOpts, err
		}
	}
	return g.buildOpts, g.buildOpts.Validate()
}

func (g *FyneGUI) writeOptions(opts RenderOptions) {
	g.loading = true
	g.opts = opts
	for _, write := range g.writeFields {
		write()
	}
	g.loading = false
	g.changed()
}

func (g *FyneGUI) changed() {
	if g.loading || g.status == nil {
		return
	}
	g.dirty = true
	g.updateButtons()
	if g.result != nil {
		g.status.SetText("Настройки изменены. Нажмите «Преобразовать», чтобы обновить превью и схему.")
	}
}

func (g *FyneGUI) updateButtons() {
	if g.convertButton == nil {
		return
	}
	for _, control := range g.controls {
		if g.busy {
			control.Disable()
		} else {
			control.Enable()
		}
	}
	if g.syncBlockControls != nil {
		g.syncBlockControls()
	}
	if g.busy {
		g.convertButton.Disable()
	} else {
		g.convertButton.Enable()
	}
	ready := g.result != nil && !g.dirty && !g.busy
	if ready {
		g.pngButton.Enable()
		g.editorButton.Enable()
	} else {
		g.pngButton.Disable()
		g.editorButton.Disable()
	}
	if ready && g.resultOpts.PaletteFile == "" {
		g.nbtButton.Enable()
		g.zipButton.Enable()
	} else {
		g.nbtButton.Disable()
		g.zipButton.Disable()
	}
}

func (g *FyneGUI) matchPreset(opts RenderOptions) {
	for _, name := range PresetNames() {
		candidate, _ := PresetOptions(name)
		candidate = preservePresetAssets(candidate, opts)
		if candidate == opts {
			g.preset.SetSelected(name)
			return
		}
	}
	g.presetInfo.SetText("Загружены пользовательские настройки. Пресет заменит обработку после нажатия «Применить».")
}

func (g *FyneGUI) applyPreset() {
	if g.busy {
		return
	}
	current, err := g.readOptions()
	if err != nil {
		g.showError(err)
		return
	}
	opts, err := PresetOptions(g.preset.Selected)
	if err != nil {
		g.showError(err)
		return
	}
	g.writeOptions(preservePresetAssets(opts, current))
	g.presetInfo.SetText(PresetDescription(g.preset.Selected))
}

func (g *FyneGUI) showError(err error) {
	g.status.SetText(err.Error())
	dialog.ShowError(err, g.window)
}

func (g *FyneGUI) startWork(message string, work func() error, done func()) {
	if g.busy {
		return
	}
	g.busy = true
	g.updateButtons()
	g.status.SetText(message)
	g.progress.Show()
	g.progress.Start()
	go func() {
		err := work()
		if g.closed.Load() {
			return
		}
		fyne.Do(func() {
			if g.closed.Load() {
				return
			}
			g.busy = false
			g.progress.Stop()
			g.progress.Hide()
			if err != nil {
				g.showError(err)
			} else if done != nil {
				done()
			}
			g.updateButtons()
		})
	}()
}

func (g *FyneGUI) openFile(title string, extensions []string, selected func(string)) {
	if g.busy {
		return
	}
	fd := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
		if err != nil {
			g.showError(err)
			return
		}
		if reader == nil {
			return
		}
		path := reader.URI().Path()
		scheme := reader.URI().Scheme()
		reader.Close()
		if scheme != "file" {
			g.showError(fmt.Errorf("выберите локальный файл"))
			return
		}
		selected(path)
	}, g.window)
	fd.SetTitleText(title)
	if len(extensions) > 0 {
		fd.SetFilter(storage.NewExtensionFileFilter(extensions))
	}
	fd.Show()
}

func (g *FyneGUI) openImageDialog() {
	g.openFile("Открыть изображение", []string{".png", ".jpg", ".jpeg", ".webp", ".gif", ".bmp", ".tif", ".tiff"}, func(path string) {
		g.path.SetText(path)
		g.convertImage()
	})
}

func (g *FyneGUI) convertImage() {
	if g.busy {
		return
	}
	path := strings.TrimSpace(g.path.Text)
	if path == "" {
		g.showError(fmt.Errorf("выберите исходное изображение"))
		return
	}
	opts, err := g.readOptions()
	if err != nil {
		g.showError(err)
		return
	}
	build, err := g.readBuild()
	if err != nil {
		g.showError(err)
		return
	}
	var result *PreviewResult
	var structure *Structure
	var buildErr error
	generated := g.generatedSource
	g.startWork("Преобразование изображения…", func() error {
		img, err := studioSourceImage(path, generated)
		if err != nil {
			return err
		}
		result, err = RenderPreview(img, opts)
		if err != nil {
			return err
		}
		structure, buildErr = BuildStructure(result, build)
		return nil
	}, func() {
		g.showResult(result, opts, path, structure, buildErr)
	})
}

func (g *FyneGUI) showResult(result *PreviewResult, opts RenderOptions, path string, structure *Structure, buildErr error) {
	g.result, g.resultOpts, g.resultSource, g.structure = result, opts, path, structure
	g.dirty = false
	g.showImage(g.sourcePane, result.Original)
	g.showImage(g.resultPane, result.Image)
	mask := image.NewGray(result.Image.Bounds())
	for i, on := range result.NoDitherMask {
		if on {
			mask.Pix[i] = 255
		}
	}
	g.showImage(g.maskPane, mask)
	g.dimensions.SetText(fmt.Sprintf("%d × %d пикселей · %d × %d карт",
		result.Image.Bounds().Dx(), result.Image.Bounds().Dy(), opts.MapsX, opts.MapsY))
	if buildErr != nil {
		g.showImage(g.mapPane, nil)
		g.materials.SetText(buildErr.Error())
		g.status.SetText("Превью готово. Схема: " + buildErr.Error())
	} else {
		g.showBuild(structure)
		g.status.SetText("Готово. Можно сохранить PNG, целую схему NBT или отдельные карты.")
	}
	if path == generatedQRSource && g.qrOptions.Text != "" && buildErr == nil {
		g.status.SetText(qrReadabilityStatus(result, structure, g.qrOptions.Text))
	}
}

func (g *FyneGUI) showBuild(s *Structure) {
	g.showImage(g.mapPane, s.MapImage)
	var text strings.Builder
	fmt.Fprintf(&text, "РАЗМЕРЫ И МАТЕРИАЛЫ\n\nСхема X × Y × Z: %d × %d × %d\nВсего блоков: %d\nЦветная поверхность: %d\nСеверный ряд: %d\nОпоры: %d\n\n",
		s.Info.Size[0], s.Info.Size[1], s.Info.Size[2], s.Info.TotalBlocks, s.Info.SurfaceBlocks, s.Info.NorthBlocks, s.Info.SupportBlocks)
	fmt.Fprintf(&text, "Режим: %s\n\n", buildModeLabel(s.Info.BuildMode))
	for _, m := range s.Info.Materials {
		fmt.Fprintf(&text, "%s: %d (%d стаков + %d)\n", strings.TrimPrefix(m.Block, "minecraft:"), m.Count, m.Stacks, m.Remainder)
	}
	fmt.Fprintf(&text, "\nРАЗМЕЩЕНИЕ\n\n%s\n\nСохраните .nbt в папку schematics вашего Minecraft. В Litematica нажмите M, затем Load Schematics и загрузите файл.\n\n«Карта в игре» учитывает целочисленную палитру Minecraft; некоторые каналы отличаются от PNG на 1. Используйте обычный мир и карты масштаба 0.\n\nНастройки постройки применяются заново при экспорте NBT.", s.Info.Placement)
	g.materials.SetText(text.String())
}

func saveTargetPath(value, extension string) (string, error) {
	path := strings.TrimSpace(value)
	if path == "" || strings.ContainsRune(path, 0) {
		return "", fmt.Errorf("укажите путь к файлу")
	}
	if filepath.Ext(path) == "" {
		path += extension
	}
	if !strings.EqualFold(filepath.Ext(path), extension) {
		return "", fmt.Errorf("нужно расширение %s", extension)
	}
	return filepath.Abs(path)
}

func (g *FyneGUI) saveDialog(title, extension string, selected func(string)) {
	if g.busy {
		return
	}
	stem := strings.TrimSuffix(filepath.Base(g.resultSource), filepath.Ext(g.resultSource))
	if g.resultSource == generatedQRSource {
		stem = "qr-code"
	}
	if stem == "" || stem == "." {
		stem = "mapart"
	}
	pathEntry := widget.NewEntry()
	pathEntry.SetText(filepath.Join("output", stem+extension))
	pathEntry.Validator = func(value string) error {
		_, err := saveTargetPath(value, extension)
		return err
	}
	browse := widget.NewButtonWithIcon("Выбрать папку…", theme.FolderOpenIcon(), func() {
		fd := dialog.NewFolderOpen(func(uri fyne.ListableURI, err error) {
			if err != nil {
				g.showError(err)
				return
			}
			if uri != nil && uri.Scheme() == "file" {
				pathEntry.SetText(filepath.Join(uri.Path(), filepath.Base(pathEntry.Text)))
			}
		}, g.window)
		fd.SetTitleText("Папка для сохранения")
		fd.Show()
	})
	fd := dialog.NewForm(title, "Сохранить", "Отмена", []*widget.FormItem{
		widget.NewFormItem("Файл", pathEntry),
		widget.NewFormItem("", browse),
	}, func(confirmed bool) {
		if !confirmed {
			return
		}
		path, err := saveTargetPath(pathEntry.Text, extension)
		if err != nil {
			g.showError(err)
			return
		}
		info, err := os.Stat(path)
		if err == nil {
			if info.IsDir() {
				g.showError(fmt.Errorf("выберите файл, а не папку"))
				return
			}
			overwrite := dialog.NewConfirm("Заменить файл?", path, func(ok bool) {
				if ok {
					selected(path)
				}
			}, g.window)
			overwrite.SetConfirmText("Заменить")
			overwrite.SetDismissText("Отмена")
			overwrite.Show()
		} else if os.IsNotExist(err) {
			selected(path)
		} else {
			g.showError(err)
		}
	}, g.window)
	fd.Resize(fyne.NewSize(700, 220))
	fd.Show()
}

func (g *FyneGUI) exportPNG() {
	if g.busy || g.result == nil || g.dirty {
		return
	}
	g.saveDialog("Сохранить превью", ".png", func(path string) {
		result, opts := g.result, g.resultOpts
		g.startWork("Сохранение PNG…", func() error { return savePreviewResult(result, path, opts, false) },
			func() { g.status.SetText("Сохранено: " + path) })
	})
}

func (g *FyneGUI) exportNBT(tiles bool) {
	if g.busy || g.result == nil || g.dirty || g.resultOpts.PaletteFile != "" {
		return
	}
	build, err := g.readBuild()
	if err != nil {
		g.showError(err)
		return
	}
	title, extension := "Сохранить схему для Litematica", ".nbt"
	if tiles {
		title, extension = "Сохранить отдельные карты", ".zip"
	}
	g.saveDialog(title, extension, func(path string) {
		result := g.result
		var structure *Structure
		g.startWork("Расчёт высот и сохранение схемы…", func() error {
			if tiles {
				return ExportStructureTiles(path, result, build)
			}
			var err error
			structure, err = BuildStructure(result, build)
			if err != nil {
				return err
			}
			return ExportStructure(path, structure)
		}, func() {
			if structure != nil {
				g.structure = structure
				g.showBuild(structure)
			}
			g.status.SetText("Сохранено: " + path)
		})
	})
}

func (g *FyneGUI) loadProfile() {
	g.openFile("Загрузить настройки", []string{".json"}, func(path string) {
		base, err := PresetOptions(g.preset.Selected)
		if err != nil {
			g.showError(err)
			return
		}
		opts, err := LoadOptions(path, base)
		if err != nil {
			g.showError(err)
			return
		}
		g.writeOptions(opts)
		g.matchPreset(opts)
		g.status.SetText("Профиль загружен. Нажмите «Преобразовать».")
	})
}

func (g *FyneGUI) saveProfile() {
	if g.busy {
		return
	}
	opts, err := g.readOptions()
	if err != nil {
		g.showError(err)
		return
	}
	g.saveDialog("Сохранить настройки", ".json", func(path string) {
		if err := SaveOptions(path, opts); err != nil {
			g.showError(err)
			return
		}
		g.status.SetText("Профиль сохранён: " + path)
	})
}

func (g *FyneGUI) showAbout() {
	dialog.ShowInformation("MapArt Studio",
		"Обработка выполняется на вашем компьютере.\n\nПресеты и JSON-профили общие с командным режимом и Windows GUI.\n\nNBT: встроенные палитры шерсти и терракоты, перепады высоты для трёх оттенков. Экспортируются материалы CSV, описание размещения и превью карты.\n\nКоманды и ограничения — в README.md рядом с программой.", g.window)
}
