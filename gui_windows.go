//go:build windows

package main

import (
	"fmt"
	"image"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"syscall"
	"unsafe"

	"github.com/lxn/walk"
	d "github.com/lxn/walk/declarative"
)

var nativeGUIRequested bool

type nativeApp struct {
	window                                        *walk.MainWindow
	path                                          *walk.LineEdit
	preset                                        *walk.ComboBox
	presetInfo                                    *walk.TextLabel
	status, dimensions                            *walk.Label
	settings                                      *walk.TabWidget
	previewTabs                                   *walk.TabWidget
	progress                                      *walk.ProgressBar
	renderButton, pngButton, nbtButton, zipButton *walk.PushButton
	editorButton                                  *walk.PushButton
	originalView, previewView, mapView, maskView  *walk.ImageView
	buildText                                     *walk.TextEdit
	zoom                                          *walk.CheckBox
	supports                                      *walk.CheckBox
	maxHeight, dataVersion                        *walk.NumberEdit
	options                                       RenderOptions
	readFields                                    []func()
	writeFields                                   []func()
	syncBlockControls                             func()
	loading, busy, dirty                          bool
	closed                                        atomic.Bool
	result                                        *PreviewResult
	resultOptions                                 RenderOptions
	resultSource                                  string
	structure                                     *Structure
	qrOptions                                     QRCodeOptions
	generatedSource                               image.Image
	bitmaps                                       map[*walk.ImageView]*walk.Bitmap
}

func runGUI(opts RenderOptions, path string, build BuildOptions) error {
	nativeGUIRequested = true
	hideOwnedConsole()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	a := &nativeApp{options: opts, loading: true, bitmaps: make(map[*walk.ImageView]*walk.Bitmap)}
	pages := a.optionPages()
	pages = append(pages, d.TabPage{Title: "Постройка", Layout: d.VBox{}, Children: []d.Widget{
		a.modeChoice(),
		d.TextLabel{Text: "2D — плоская поверхность, один оттенок на блок.\n3D — лестница, три оттенка за счёт высот.\nСеверный служебный ряд добавляется автоматически."},
		d.CheckBox{AssignTo: &a.supports, Text: "Опоры из булыжника под перепадами", Checked: build.Supports},
		d.Label{Text: "Максимальная высота схемы, блоков"},
		d.NumberEdit{AssignTo: &a.maxHeight, Value: float64(build.MaxHeight), MinValue: 2, MaxValue: 4096, Decimals: 0, SpinButtonsVisible: true},
		d.Label{Text: "DataVersion NBT (5023 = Java 26.3)"},
		d.NumberEdit{AssignTo: &a.dataVersion, Value: float64(build.DataVersion), MinValue: 1519, MaxValue: 2147483647, Decimals: 0},
		d.TextLabel{Text: "5023 = Java 26.3 (полный каталог). 3465 = Java 1.20.1 для старых материалов. Произвольная RGB-палитра не поддерживается при экспорте блоков.\n\nZIP по картам содержит независимые постройки 128×128. Обязательные опоры под сыпучими и тонкими блоками сохраняются при отключении обычных опор."},
		d.VSpacer{},
	}})
	window := d.MainWindow{AssignTo: &a.window, Title: "MapArt Studio", MinSize: d.Size{Width: 1000, Height: 680}, Size: d.Size{Width: 1320, Height: 850}, Font: d.Font{Family: "Segoe UI", PointSize: 10}, Layout: d.VBox{Margins: d.Margins{Left: 16, Top: 12, Right: 16, Bottom: 12}, Spacing: 10},
		OnDropFiles: func(paths []string) {
			if !a.busy && len(paths) > 0 {
				a.path.SetText(paths[0])
				a.render()
			}
		},
		Children: []d.Widget{
			d.Composite{Layout: d.HBox{MarginsZero: true}, Children: []d.Widget{
				d.Label{Text: "MAPART", Font: d.Font{Family: "Segoe UI", PointSize: 19, Bold: true}, TextColor: walk.RGB(27, 79, 120)},
				d.Label{Text: "Картинка → карта → постройка", TextColor: walk.RGB(90, 100, 115)}, d.HSpacer{},
				d.PushButton{Text: "QR-код…", OnClicked: a.qrGenerator},
				d.PushButton{Text: "О программе", OnClicked: a.about},
			}},
			d.Composite{Layout: d.HBox{MarginsZero: true}, Children: []d.Widget{
				d.LineEdit{AssignTo: &a.path, Name: "source_path", CueBanner: "Выберите картинку или перетащите её в окно", Text: path, OnTextChanged: a.changed},
				d.PushButton{Text: "Редактор исходника…", OnClicked: a.editSource},
				d.PushButton{Text: "Открыть картинку…", OnClicked: a.openImage},
			}},
			d.HSplitter{Children: []d.Widget{
				d.Composite{MinSize: d.Size{Width: 385, Height: 0}, MaxSize: d.Size{Width: 470, Height: 0}, Layout: d.VBox{MarginsZero: true, Spacing: 8}, Children: []d.Widget{
					d.Composite{Layout: d.HBox{MarginsZero: true}, Children: []d.Widget{
						d.Label{Text: "Пресет"}, d.ComboBox{AssignTo: &a.preset, Model: PresetNames(), CurrentIndex: 0}, d.PushButton{Text: "Применить", OnClicked: a.applyPreset},
					}},
					d.TextLabel{AssignTo: &a.presetInfo, Text: PresetDescription("balanced")},
					d.TabWidget{AssignTo: &a.settings, Pages: pages, StretchFactor: 1},
					d.Composite{Layout: d.HBox{MarginsZero: true}, Children: []d.Widget{
						d.PushButton{Text: "Загрузить JSON…", OnClicked: a.loadProfile}, d.PushButton{Text: "Сохранить JSON…", OnClicked: a.saveProfile},
					}},
					d.PushButton{AssignTo: &a.renderButton, Name: "render", Text: "Преобразовать", MinSize: d.Size{Width: 0, Height: 40}, Font: d.Font{Family: "Segoe UI", PointSize: 11, Bold: true}, OnClicked: a.render},
				}},
				d.Composite{Layout: d.VBox{MarginsZero: true, Spacing: 8}, StretchFactor: 3, Children: []d.Widget{
					d.Composite{Layout: d.HBox{MarginsZero: true}, Children: []d.Widget{
						d.Label{AssignTo: &a.dimensions, Text: "Превью появится после преобразования"}, d.HSpacer{},
						d.CheckBox{AssignTo: &a.zoom, Text: "Размер 1:1", OnCheckedChanged: a.setZoom},
					}},
					d.TabWidget{AssignTo: &a.previewTabs, StretchFactor: 1, Pages: []d.TabPage{
						{Title: "Сравнение", Layout: d.HBox{MarginsZero: true}, Children: []d.Widget{
							d.GroupBox{Title: "Исходник", Layout: d.VBox{Margins: d.Margins{Left: 4, Top: 4, Right: 4, Bottom: 4}}, Children: []d.Widget{a.imagePane(&a.originalView)}},
							d.GroupBox{Title: "Мап арт", Layout: d.VBox{Margins: d.Margins{Left: 4, Top: 4, Right: 4, Bottom: 4}}, Children: []d.Widget{a.imagePane(&a.previewView)}},
						}},
						{Title: "Карта в игре", Layout: d.VBox{}, Children: []d.Widget{a.imagePane(&a.mapView)}},
						{Title: "Маска фона", Layout: d.VBox{}, Children: []d.Widget{a.imagePane(&a.maskView)}},
						{Title: "Материалы и размещение", Layout: d.VBox{}, Children: []d.Widget{d.TextEdit{AssignTo: &a.buildText, ReadOnly: true, VScroll: true, Text: "После преобразования здесь появятся размеры схемы, материалы и инструкция по размещению."}}},
					}},
					d.Composite{Layout: d.HBox{MarginsZero: true}, Children: []d.Widget{
						d.PushButton{AssignTo: &a.editorButton, Text: "Редактор карты…", Enabled: false, OnClicked: a.editMap},
						d.PushButton{AssignTo: &a.pngButton, Text: "Сохранить PNG…", Enabled: false, OnClicked: a.savePNG},
						d.PushButton{AssignTo: &a.nbtButton, Text: "Схема .nbt…", Enabled: false, OnClicked: func() { a.saveNBT(false) }},
						d.PushButton{AssignTo: &a.zipButton, Text: "ZIP по картам…", Enabled: false, OnClicked: func() { a.saveNBT(true) }},
					}},
				}},
			}},
			d.ProgressBar{AssignTo: &a.progress, MaxSize: d.Size{Width: 0, Height: 6}},
			d.Label{AssignTo: &a.status, Text: "Выберите изображение. Все операции выполняются локально."},
		},
	}
	if err := window.Create(); err != nil {
		return err
	}
	defer a.window.Dispose()
	a.window.Closing().Attach(func(canceled *bool, reason walk.CloseReason) { a.closed.Store(true) })
	a.writeOptions(opts)
	a.loading = false
	matched := false
	for i, name := range PresetNames() {
		candidate, _ := PresetOptions(name)
		candidate = preservePresetAssets(candidate, opts)
		if candidate == opts {
			a.preset.SetCurrentIndex(i)
			a.presetInfo.SetText(PresetDescription(name))
			matched = true
			break
		}
	}
	if !matched {
		a.presetInfo.SetText("Загружены пользовательские настройки. Пресет заменит обработку после нажатия «Применить».")
	}
	a.preset.CurrentIndexChanged().Attach(func() { name := a.preset.Text(); a.presetInfo.SetText(PresetDescription(name)) })
	centerNativeWindow(a.window)
	a.window.Run()
	for _, bitmap := range a.bitmaps {
		bitmap.Dispose()
	}
	return nil
}

func (a *nativeApp) imagePane(target **walk.ImageView) d.Widget {
	return d.ScrollView{Layout: d.VBox{MarginsZero: true}, Children: []d.Widget{
		d.ImageView{AssignTo: target, Mode: d.ImageViewModeZoom, Background: d.SolidColorBrush{Color: walk.RGB(235, 238, 242)}, MinSize: d.Size{Width: 100, Height: 100}},
	}}
}

// Bind native controls directly to the same options used by the CLI.
func (a *nativeApp) number(label string, get func() float64, set func(float64), low, high float64, decimals int) d.Widget {
	var field *walk.NumberEdit
	a.readFields = append(a.readFields, func() { set(field.Value()) })
	a.writeFields = append(a.writeFields, func() { field.SetValue(get()) })
	inc := 1.0
	if decimals > 0 {
		inc = .05
	}
	return d.Composite{Layout: d.HBox{MarginsZero: true}, Children: []d.Widget{d.Label{Text: label, MinSize: d.Size{Width: 190, Height: 0}}, d.NumberEdit{AssignTo: &field, MinValue: low, MaxValue: high, Decimals: decimals, Increment: inc, SpinButtonsVisible: true, OnValueChanged: a.changed}}}
}
func (a *nativeApp) choice(label string, get func() string, set func(string), values []string) d.Widget {
	var field *walk.ComboBox
	a.readFields = append(a.readFields, func() { set(field.Text()) })
	a.writeFields = append(a.writeFields, func() {
		for i, v := range values {
			if v == get() {
				field.SetCurrentIndex(i)
				break
			}
		}
	})
	return d.Composite{Layout: d.HBox{MarginsZero: true}, Children: []d.Widget{d.Label{Text: label, MinSize: d.Size{Width: 190, Height: 0}}, d.ComboBox{AssignTo: &field, Model: values, OnCurrentIndexChanged: a.changed}}}
}
func (a *nativeApp) check(label string, get func() bool, set func(bool)) d.Widget {
	var field *walk.CheckBox
	a.readFields = append(a.readFields, func() { set(field.Checked()) })
	a.writeFields = append(a.writeFields, func() { field.SetChecked(get()) })
	return d.CheckBox{AssignTo: &field, Text: label, OnCheckedChanged: func() { set(field.Checked()); a.changed() }}
}
func (a *nativeApp) text(label string, get func() string, set func(string), file bool) d.Widget {
	var field *walk.LineEdit
	a.readFields = append(a.readFields, func() { set(strings.TrimSpace(field.Text())) })
	a.writeFields = append(a.writeFields, func() { field.SetText(get()) })
	children := []d.Widget{d.LineEdit{AssignTo: &field, OnTextChanged: a.changed}}
	if file {
		children = append(children, d.PushButton{Text: "…", MaxSize: d.Size{Width: 32, Height: 0}, OnClicked: func() {
			if a.busy {
				return
			}
			dlg := walk.FileDialog{Title: label, Filter: "Все файлы|*.*"}
			ok, err := dlg.ShowOpen(a.window)
			if err != nil {
				a.showError(err)
			} else if ok {
				field.SetText(dlg.FilePath)
			}
		}})
	}
	return d.Composite{Layout: d.VBox{MarginsZero: true, Spacing: 3}, Children: []d.Widget{d.Label{Text: label}, d.Composite{Layout: d.HBox{MarginsZero: true}, Children: children}}}
}

func (a *nativeApp) optionPages() []d.TabPage {
	o := &a.options
	page := func(title string, children ...d.Widget) d.TabPage {
		children = append(children, d.VSpacer{})
		return d.TabPage{Title: title, Layout: d.VBox{MarginsZero: true}, Children: []d.Widget{d.ScrollView{HorizontalFixed: true, Layout: d.VBox{Spacing: 9}, Children: children}}}
	}
	return []d.TabPage{
		a.blockPage(),
		page("Размер",
			a.number("Карт по горизонтали", func() float64 { return float64(o.MapsX) }, func(v float64) { o.MapsX = int(v) }, 1, 256, 0),
			a.number("Карт по вертикали", func() float64 { return float64(o.MapsY) }, func(v float64) { o.MapsY = int(v) }, 1, 256, 0),
			d.TextLabel{Text: "Одна карта — 128×128 блоков. 5×3 = 640×384. Всего не более 256 карт."},
			a.choice("Вписывание", func() string { return o.Fit }, func(v string) { o.Fit = v }, []string{"stretch", "contain", "cover"}),
			d.TextLabel{Text: "stretch — растянуть; contain — поля; cover — обрезка по центру."},
			a.choice("Масштабирование", func() string { return o.Resample }, func(v string) { o.Resample = v }, []string{"bicubic", "bilinear", "nearest"}),
			a.text("Обрезка: x,y,ширина,высота (можно пусто)", func() string { return o.Crop }, func(v string) { o.Crop = v }, false),
			a.text("Цвет полей и прозрачности #RRGGBB", func() string { return o.Matte }, func(v string) { o.Matte = v }, false)),
		page("Дизеринг",
			a.choice("Режим", func() string { return o.Dither }, func(v string) { o.Dither = v }, []string{"fs", "ordered", "none"}),
			a.number("Сила", func() float64 { return o.DitherStrength }, func(v float64) { o.DitherStrength = v }, 0, 1, 2),
			a.number("Порог Delta E", func() float64 { return o.Threshold }, func(v float64) { o.Threshold = v }, 0, 1000000, 2),
			a.choice("Подбор цвета", func() string { return o.ColorMetric }, func(v string) { o.ColorMetric = v }, []string{"lab", "rgb"}),
			a.check("Приглушать цветные точки в дымке", func() bool { return o.CleanHaze }, func(v bool) { o.CleanHaze = v }),
			d.TextLabel{Text: "fs — рассеивание ошибки; ordered — регулярная сетка; none — без дизеринга.\n\nПодавление точек в дымке действует в fs и может ослабить пастель."},
			a.check("Только нейтральные цвета", func() bool { return o.Monochrome }, func(v bool) { o.Monochrome = v })),
		page("Цвет",
			a.check("Полная палитра блоков (Java 26.3)", func() bool { return o.AllBlocks }, func(v bool) { o.AllBlocks = v }),
			a.check("Добавить терракоту (96 оттенков, PNG и NBT)", func() bool { return o.Terracotta }, func(v bool) { o.Terracotta = v }),
			a.check("Добавить природные блоки (+18 оттенков, PNG и NBT)", func() bool { return o.NaturalBlocks }, func(v bool) { o.NaturalBlocks = v }),
			a.number("Яркость", func() float64 { return o.Brightness }, func(v float64) { o.Brightness = v }, -100, 100, 1),
			a.number("Контраст", func() float64 { return o.Contrast }, func(v float64) { o.Contrast = v }, .1, 3, 2),
			a.number("Насыщенность", func() float64 { return o.Saturation }, func(v float64) { o.Saturation = v }, 0, 3, 2),
			a.number("Гамма", func() float64 { return o.Gamma }, func(v float64) { o.Gamma = v }, .2, 3, 2),
			a.number("Температура", func() float64 { return o.Temperature }, func(v float64) { o.Temperature = v }, -100, 100, 1),
			a.number("Резкость", func() float64 { return o.Sharpen }, func(v float64) { o.Sharpen = v }, 0, 2, 2),
			a.number("Подавление шума", func() float64 { return o.Denoise }, func(v float64) { o.Denoise = v }, 0, 1, 2),
			a.check("Автоматический контраст", func() bool { return o.AutoContrast }, func(v bool) { o.AutoContrast = v })),
		page("Маски",
			a.choice("Ровный фон", func() string { return o.Background }, func(v string) { o.Background = v }, []string{"auto", "off", "color"}),
			a.text("Цвет фона #RRGGBB (для режима color)", func() string { return o.BackgroundColor }, func(v string) { o.BackgroundColor = v }, false),
			a.number("Допуск фона", func() float64 { return o.BackgroundTolerance }, func(v float64) { o.BackgroundTolerance = v }, 0, 150, 1),
			a.check("Убирать дизеринг со светлого неба", func() bool { return o.SkyCleanup }, func(v bool) { o.SkyCleanup = v }),
			a.text("Маска без дизеринга (белым выделить фон)", func() string { return o.NoDitherMask }, func(v string) { o.NoDitherMask = v }, true),
			a.text("Маска деталей (белым вернуть дизеринг)", func() string { return o.DetailMask }, func(v string) { o.DetailMask = v }, true),
			a.text("Своя RGB-палитра JSON (только PNG)", func() string { return o.PaletteFile }, func(v string) { o.PaletteFile = v }, true)),
	}
}

func (a *nativeApp) readOptions() (RenderOptions, error) {
	for _, f := range a.readFields {
		f()
	}
	return a.options, ValidateOptions(a.options)
}
func (a *nativeApp) writeOptions(opts RenderOptions) {
	a.loading = true
	a.options = opts
	for _, f := range a.writeFields {
		f()
	}
	a.loading = false
	a.changed()
}
func (a *nativeApp) buildOptions() BuildOptions {
	return BuildOptions{a.supports.Checked(), int(a.maxHeight.Value()), int(a.dataVersion.Value())}
}
func (a *nativeApp) changed() {
	if a.loading || a.status == nil {
		return
	}
	a.dirty = true
	a.updateButtons()
	if a.result != nil {
		a.status.SetText("Настройки изменены. Нажмите «Преобразовать», чтобы обновить превью и схему.")
	}
}
func (a *nativeApp) updateButtons() {
	if a.renderButton == nil {
		return
	}
	a.renderButton.SetEnabled(!a.busy)
	a.settings.SetEnabled(!a.busy)
	a.path.SetEnabled(!a.busy)
	if a.syncBlockControls != nil {
		a.syncBlockControls()
	}
	ready := a.result != nil && !a.dirty && !a.busy
	a.pngButton.SetEnabled(ready)
	a.editorButton.SetEnabled(ready)
	buildable := ready && a.resultOptions.PaletteFile == ""
	a.nbtButton.SetEnabled(buildable)
	a.zipButton.SetEnabled(buildable)
}
func (a *nativeApp) showError(err error) {
	a.status.SetText(err.Error())
	walk.MsgBox(a.window, "MapArt — ошибка", err.Error(), walk.MsgBoxIconError)
}
func (a *nativeApp) startWork(message string, work func() error, done func()) {
	if a.busy {
		return
	}
	a.busy = true
	a.updateButtons()
	a.progress.SetMarqueeMode(true)
	a.status.SetText(message)
	go func() {
		err := work()
		if a.closed.Load() {
			return
		}
		a.window.Synchronize(func() {
			if a.closed.Load() {
				return
			}
			a.busy = false
			a.progress.SetMarqueeMode(false)
			a.updateButtons()
			if err != nil {
				a.showError(err)
			} else if done != nil {
				done()
			}
		})
	}()
}
func (a *nativeApp) openImage() {
	if a.busy {
		return
	}
	dlg := walk.FileDialog{Title: "Открыть изображение", Filter: "Изображения|*.png;*.jpg;*.jpeg;*.webp;*.bmp;*.gif;*.tif;*.tiff|Все файлы|*.*"}
	ok, err := dlg.ShowOpen(a.window)
	if err != nil {
		a.showError(err)
	} else if ok {
		a.path.SetText(dlg.FilePath)
		a.render()
	}
}
func (a *nativeApp) applyPreset() {
	if a.busy {
		return
	}
	current, err := a.readOptions()
	if err != nil {
		a.showError(err)
		return
	}
	opts, err := PresetOptions(a.preset.Text())
	if err != nil {
		a.showError(err)
		return
	}
	a.writeOptions(preservePresetAssets(opts, current))
	a.presetInfo.SetText(PresetDescription(a.preset.Text()))
}
func (a *nativeApp) render() {
	if a.busy {
		return
	}
	path := strings.TrimSpace(a.path.Text())
	if path == "" {
		a.showError(fmt.Errorf("выберите исходное изображение"))
		return
	}
	opts, err := a.readOptions()
	if err != nil {
		a.showError(err)
		return
	}
	build := a.buildOptions()
	if err = build.Validate(); err != nil {
		a.showError(err)
		return
	}
	var result *PreviewResult
	var structure *Structure
	var buildErr error
	generated := a.generatedSource
	a.startWork("Преобразование изображения…", func() error {
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
		a.showResult(result, opts, path, structure, buildErr)
	})
}

func (a *nativeApp) showResult(result *PreviewResult, opts RenderOptions, path string, structure *Structure, buildErr error) {
	a.result, a.resultOptions, a.resultSource, a.structure = result, opts, path, structure
	a.dirty = false
	if err := a.showImage(a.originalView, result.Original); err != nil {
		a.showError(err)
		return
	}
	if err := a.showImage(a.previewView, result.Image); err != nil {
		a.showError(err)
		return
	}
	mask := image.NewGray(result.Image.Bounds())
	for i, on := range result.NoDitherMask {
		if on {
			mask.Pix[i] = 255
		}
	}
	if err := a.showImage(a.maskView, mask); err != nil {
		a.showError(err)
		return
	}
	a.dimensions.SetText(fmt.Sprintf("%d × %d пикселей · %d × %d карт", result.Image.Bounds().Dx(), result.Image.Bounds().Dy(), opts.MapsX, opts.MapsY))
	if buildErr != nil {
		a.clearImage(a.mapView)
		a.buildText.SetText(buildErr.Error())
		a.status.SetText("Превью готово. Схема: " + buildErr.Error())
	} else {
		a.showBuild(structure)
		a.status.SetText("Готово. Можно сохранить PNG, целую схему NBT или отдельные карты.")
	}
	if path == generatedQRSource && a.qrOptions.Text != "" && buildErr == nil {
		a.status.SetText(qrReadabilityStatus(result, structure, a.qrOptions.Text))
	}
	a.setZoom()
	a.updateButtons()
}
func (a *nativeApp) clearImage(view *walk.ImageView) {
	view.SetImage(nil)
	if old := a.bitmaps[view]; old != nil {
		old.Dispose()
		delete(a.bitmaps, view)
	}
}
func (a *nativeApp) showImage(view *walk.ImageView, img image.Image) error {
	bitmap, err := walk.NewBitmapFromImageForDPI(img, view.DPI())
	if err != nil {
		return err
	}
	if err = view.SetImage(bitmap); err != nil {
		bitmap.Dispose()
		return err
	}
	if old := a.bitmaps[view]; old != nil {
		old.Dispose()
	}
	a.bitmaps[view] = bitmap
	return nil
}
func (a *nativeApp) setZoom() {
	if a.zoom == nil {
		return
	}
	for _, view := range []*walk.ImageView{a.originalView, a.previewView, a.mapView, a.maskView} {
		if view == nil {
			continue
		}
		bitmap := a.bitmaps[view]
		if a.zoom.Checked() && bitmap != nil {
			view.SetMode(walk.ImageViewModeCorner)
			view.SetMinMaxSize(bitmap.Size(), walk.Size{})
		} else {
			view.SetMinMaxSize(walk.Size{Width: 100, Height: 100}, walk.Size{})
			view.SetMode(walk.ImageViewModeZoom)
		}
	}
}
func (a *nativeApp) showBuild(s *Structure) {
	if err := a.showImage(a.mapView, s.MapImage); err != nil {
		a.showError(err)
	}
	var text strings.Builder
	fmt.Fprintf(&text, "РАЗМЕРЫ И МАТЕРИАЛЫ\r\n\r\nСхема X × Y × Z: %d × %d × %d\r\nВсего блоков: %d\r\nЦветная поверхность: %d\r\nСеверный ряд: %d\r\nОпоры: %d\r\n\r\n", s.Info.Size[0], s.Info.Size[1], s.Info.Size[2], s.Info.TotalBlocks, s.Info.SurfaceBlocks, s.Info.NorthBlocks, s.Info.SupportBlocks)
	fmt.Fprintf(&text, "Режим: %s\r\n\r\n", buildModeLabel(s.Info.BuildMode))
	for _, m := range s.Info.Materials {
		fmt.Fprintf(&text, "%s: %d (%d стаков + %d)\r\n", strings.TrimPrefix(m.Block, "minecraft:"), m.Count, m.Stacks, m.Remainder)
	}
	fmt.Fprintf(&text, "\r\nРАЗМЕЩЕНИЕ\r\n\r\n%s\r\n\r\nСохраните .nbt в папку schematics вашего Minecraft. В Litematica откройте M → Load Schematics и загрузите файл.\r\n\r\n«Карта в игре» учитывает целочисленную палитру Minecraft; некоторые каналы отличаются от прежнего PNG на 1. Используйте обычный мир и карты масштаба 0.\r\n\r\nНастройки постройки применяются заново при экспорте NBT.", s.Info.Placement)
	a.buildText.SetText(text.String())
	a.setZoom()
}
func (a *nativeApp) saveDialog(title, extension, filter string) (string, bool) {
	stem := strings.TrimSuffix(filepath.Base(a.resultSource), filepath.Ext(a.resultSource))
	if a.resultSource == generatedQRSource {
		stem = "qr-code"
	}
	if stem == "" || stem == "." {
		stem = "mapart"
	}
	dlg := walk.FileDialog{Title: title, FilePath: stem + extension, Filter: filter}
	ok, err := dlg.ShowSave(a.window)
	if err != nil {
		a.showError(err)
		return "", false
	}
	if !ok {
		return "", false
	}
	path := dlg.FilePath
	if filepath.Ext(path) == "" {
		path += extension
	}
	if !strings.EqualFold(filepath.Ext(path), extension) {
		a.showError(fmt.Errorf("нужно расширение %s", extension))
		return "", false
	}
	return path, true
}
func (a *nativeApp) savePNG() {
	if a.busy || a.result == nil || a.dirty {
		return
	}
	path, ok := a.saveDialog("Сохранить превью", ".png", "PNG|*.png")
	if !ok {
		return
	}
	result, opts := a.result, a.resultOptions
	a.startWork("Сохранение PNG…", func() error { return savePreviewResult(result, path, opts, false) }, func() { a.status.SetText("Сохранено: " + path) })
}
func (a *nativeApp) saveNBT(tiles bool) {
	if a.busy || a.result == nil || a.dirty {
		return
	}
	build := a.buildOptions()
	if err := build.Validate(); err != nil {
		a.showError(err)
		return
	}
	ext, filter, title := ".nbt", "Схема NBT|*.nbt", "Сохранить схему для Litematica"
	if tiles {
		ext, filter, title = ".zip", "ZIP с картами NBT|*.zip", "Сохранить отдельные карты"
	}
	path, ok := a.saveDialog(title, ext, filter)
	if !ok {
		return
	}
	result := a.result
	var structure *Structure
	a.startWork("Расчёт высот и сохранение схемы…", func() error {
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
			a.structure = structure
			a.showBuild(structure)
		}
		a.status.SetText("Сохранено: " + path)
	})
}
func (a *nativeApp) loadProfile() {
	if a.busy {
		return
	}
	dlg := walk.FileDialog{Title: "Загрузить настройки", Filter: "JSON-профиль|*.json"}
	ok, err := dlg.ShowOpen(a.window)
	if err != nil {
		a.showError(err)
		return
	}
	if !ok {
		return
	}
	base, err := PresetOptions(a.preset.Text())
	if err != nil {
		a.showError(err)
		return
	}
	opts, err := LoadOptions(dlg.FilePath, base)
	if err != nil {
		a.showError(err)
		return
	}
	a.writeOptions(opts)
	a.status.SetText("Профиль загружен. Нажмите «Преобразовать».")
}
func (a *nativeApp) saveProfile() {
	if a.busy {
		return
	}
	opts, err := a.readOptions()
	if err != nil {
		a.showError(err)
		return
	}
	path, ok := a.saveDialog("Сохранить настройки", ".json", "JSON-профиль|*.json")
	if !ok {
		return
	}
	if err = SaveOptions(path, opts); err != nil {
		a.showError(err)
		return
	}
	a.status.SetText("Профиль сохранён: " + path)
}
func (a *nativeApp) about() {
	walk.MsgBox(a.window, "MapArt Studio", "Нативное приложение Windows. Обработка выполняется на вашем компьютере.\n\nПресеты и JSON-профили общие с командным режимом mapart.exe.\n\nNBT: 16 цветов шерсти и перепады высоты для трёх оттенков. Экспортируются также материалы CSV, описание размещения и превью карты.\n\nПодробные команды и ограничения — в README.md рядом с программой.", walk.MsgBoxIconInformation)
}

func reportGUIStartupError(err error) {
	if !nativeGUIRequested {
		return
	}
	// Works even if the toolkit could not create its first window.
	user32 := syscall.NewLazyDLL("user32.dll")
	message := syscall.StringToUTF16Ptr(err.Error())
	title := syscall.StringToUTF16Ptr("MapArt")
	user32.NewProc("MessageBoxW").Call(0, uintptr(unsafe.Pointer(message)), uintptr(unsafe.Pointer(title)), 0x10)
}

// Keep the default executable useful on double-click without hiding a parent
// terminal when -gui is launched from PowerShell.
func hideOwnedConsole() {
	kernel := syscall.NewLazyDLL("kernel32.dll")
	var ids [2]uint32
	count, _, _ := kernel.NewProc("GetConsoleProcessList").Call(uintptr(unsafe.Pointer(&ids[0])), 2)
	if count == 1 && ids[0] == uint32(os.Getpid()) {
		hwnd, _, _ := kernel.NewProc("GetConsoleWindow").Call()
		syscall.NewLazyDLL("user32.dll").NewProc("ShowWindow").Call(hwnd, 0)
	}
}

func centerNativeWindow(window *walk.MainWindow) {
	var area [4]int32
	user32 := syscall.NewLazyDLL("user32.dll")
	ok, _, _ := user32.NewProc("SystemParametersInfoW").Call(0x30, 0, uintptr(unsafe.Pointer(&area[0])), 0)
	if ok == 0 {
		return
	}
	width, height := int(area[2]-area[0]), int(area[3]-area[1])
	w, h := min(1320, width-24), min(850, height-24)
	window.SetBoundsPixels(walk.Rectangle{X: int(area[0]) + (width-w)/2, Y: int(area[1]) + (height-h)/2, Width: w, Height: h})
}
