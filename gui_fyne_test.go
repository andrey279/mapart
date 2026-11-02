//go:build !windows && gui

package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// Pump background completions on the test thread, matching Fyne's real UI
// event queue. The default headless driver executes them on the worker thread.
type studioQueuedDriver struct {
	fyne.Driver
	calls chan func()
}

func (d *studioQueuedDriver) DoFromGoroutine(f func(), _ bool) { d.calls <- f }

type studioQueuedApp struct {
	fyne.App
	driver *studioQueuedDriver
}

func (a studioQueuedApp) Driver() fyne.Driver { return a.driver }

func TestStudioQRCodeGeneratorAndEditableSource(t *testing.T) {
	g := newStudioTest(t)
	driver := &studioQueuedDriver{Driver: g.app.Driver(), calls: make(chan func(), 128)}
	fyne.SetCurrentApp(studioQueuedApp{App: g.app, driver: driver})
	button := findStudioTestButton(g.window.Content(), "QR-код…")
	if button == nil {
		t.Fatal("QR generator action is missing")
	}
	test.Tap(button)
	overlays := g.window.Canvas().Overlays().List()
	overlay := overlays[len(overlays)-1]
	var text *widget.Entry
	for _, object := range studioTestObjects(overlay) {
		if entry, ok := object.(*widget.Entry); ok && entry.MultiLine {
			text = entry
			break
		}
	}
	if text == nil {
		t.Fatal("QR payload field is missing")
	}
	text.SetText("https://example.com/mapart")
	update, create := findStudioTestButton(overlay, "Обновить превью"), findStudioTestButton(overlay, "Создать мап-арт")
	if update == nil || create == nil {
		t.Fatal("QR preview/apply controls are missing")
	}
	wait := func() {
		t.Helper()
		timer := time.NewTimer(15 * time.Second)
		defer timer.Stop()
		for g.busy {
			select {
			case done := <-driver.calls:
				done()
			case <-timer.C:
				t.Fatal("QR generator did not finish")
			}
		}
	}
	test.Tap(update)
	wait()
	if g.result != nil || g.generatedSource != nil {
		t.Fatal("preview replaced the main map")
	}
	if !strings.Contains(g.status.Text, "проверено") {
		t.Fatal("QR preview failed:", g.status.Text)
	}
	if dir := os.Getenv("MAPART_GUI_SCREENSHOT_DIR"); dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := savePNG(filepath.Join(dir, "qr-generator.png"), g.window.Canvas().Capture()); err != nil {
			t.Fatal(err)
		}
	}
	test.Tap(create)
	wait()
	if g.path.Text != generatedQRSource || g.generatedSource == nil || g.result == nil || g.structure == nil {
		t.Fatal("QR source was not installed")
	}
	if g.resultOpts.BuildMode != "2d" || g.pngButton.Disabled() || g.nbtButton.Disabled() || g.zipButton.Disabled() || g.editorButton.Disabled() {
		t.Fatal("QR cannot be edited/exported")
	}
	if err := VerifyQRCode(g.structure.MapImage, text.Text); err != nil {
		t.Fatal("QR in the Minecraft tab does not scan:", err)
	}
	// A generated image must still be usable after options change, and the
	// source editor must not try to open its display label as a disk path.
	g.convertImage()
	wait()
	if g.dirty || !strings.Contains(g.status.Text, "проверен") {
		t.Fatal("generated source could not be reprocessed", g.status.Text)
	}
	g.editSource()
	wait()
	overlays = g.window.Canvas().Overlays().List()
	overlay = overlays[len(overlays)-1]
	if cancel := findStudioTestButton(overlay, "Отмена"); cancel == nil {
		t.Fatal("QR source editor did not open")
	} else {
		test.Tap(cancel)
	}
	// Reopening and dismissing the generator keeps the finished art intact.
	previous := g.result
	g.qrGenerator()
	overlays = g.window.Canvas().Overlays().List()
	test.Tap(findStudioTestButton(overlays[len(overlays)-1], "Закрыть"))
	if g.result != previous || g.dirty {
		t.Fatal("cancelling QR generator changed the map")
	}
}

func TestStudioMapZoomWheelButtonsAndPixelCoordinates(t *testing.T) {
	a := test.NewApp()
	t.Cleanup(a.Quit)
	source := image.NewRGBA(image.Rect(0, 0, 200, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 200; x++ {
			source.SetRGBA(x, y, color.RGBA{uint8(x), uint8(y), 180, 255})
		}
	}
	before := append([]byte(nil), source.Pix...)
	view := newStudioEditCanvas(source, false)
	scroll, controls := studioMapZoom(view)
	scroll.Resize(fyne.NewSize(400, 300))
	var choice *widget.Select
	for _, object := range studioTestObjects(controls) {
		if selectBox, ok := object.(*widget.Select); ok {
			choice = selectBox
			break
		}
	}
	if choice == nil {
		t.Fatal("zoom selector is missing")
	}
	zoomIn, zoomOut := findStudioTestButton(controls, "+"), findStudioTestButton(controls, "−")
	if zoomIn == nil || zoomOut == nil {
		t.Fatal("zoom buttons are missing")
	}
	// Zooming in from Fit must enlarge the image even if Fit is above 100%.
	test.Tap(zoomIn)
	if view.zoom <= 2 || view.picture.Size().Width <= 400 {
		t.Fatal("zoom in shrank an image that was already enlarged by Fit")
	}
	test.Tap(zoomOut)
	if view.zoom != 2 {
		t.Fatal("zoom out did not reverse the previous step", view.zoom)
	}
	choice.SetSelected("400%")
	anchor := fyne.NewPos(scroll.Offset.X+150, scroll.Offset.Y+120)
	oldPixel, ok := view.point(anchor, false)
	if !ok {
		t.Fatal("test cursor is outside the image")
	}
	view.Scrolled(&fyne.ScrollEvent{PointEvent: fyne.PointEvent{Position: anchor}, Scrolled: fyne.Delta{DY: 1}})
	newPixel, ok := view.point(fyne.NewPos(scroll.Offset.X+150, scroll.Offset.Y+120), false)
	if view.zoom != 5 || !ok || newPixel != oldPixel {
		t.Fatalf("wheel zoom moved the pixel under the cursor: before=%v after=%v zoom=%v", oldPixel, newPixel, view.zoom)
	}
	view.Scrolled(&fyne.ScrollEvent{PointEvent: fyne.PointEvent{Position: fyne.NewPos(scroll.Offset.X+150, scroll.Offset.Y+120)}, Scrolled: fyne.Delta{DY: -1}})
	if view.zoom != 4 {
		t.Fatal("wheel down did not reduce zoom")
	}
	choice.SetSelected("25%")
	if view.picture.Size() != fyne.NewSize(50, 25) {
		t.Fatal("zoom out stretched the image back to the viewport", view.picture.Size())
	}
	if _, ok := view.point(fyne.NewPos(10, 10), false); ok {
		t.Fatal("empty margin can be painted after zooming out")
	}
	pixel, ok := view.point(fyne.NewPos(200, 150), false)
	if !ok || pixel != image.Pt(100, 50) {
		t.Fatal("brush coordinates do not match the reduced image", pixel)
	}
	view.MouseDown(&desktop.MouseEvent{PointEvent: fyne.PointEvent{Position: fyne.NewPos(200, 150)}, Button: desktop.MouseButtonPrimary})
	view.Scrolled(&fyne.ScrollEvent{PointEvent: fyne.PointEvent{Position: fyne.NewPos(200, 150)}, Scrolled: fyne.Delta{DY: 1}})
	if view.zoom != .25 {
		t.Fatal("wheel changed scale in the middle of a brush stroke")
	}
	view.DragEnd()
	choice.SetSelected("3200%")
	view.Scrolled(&fyne.ScrollEvent{PointEvent: fyne.PointEvent{Position: fyne.NewPos(scroll.Offset.X+200, scroll.Offset.Y+150)}, Scrolled: fyne.Delta{DY: 1}})
	if view.zoom != maxEditorZoom {
		t.Fatal("zoom exceeded its upper limit")
	}
	test.Tap(findStudioTestButton(controls, "Вписать"))
	if view.zoom != 0 || scroll.Direction != container.ScrollNone || scroll.Offset != (fyne.Position{}) {
		t.Fatal("Fit did not reset zoom and scrolling")
	}
	if !bytes.Equal(source.Pix, before) {
		t.Fatal("changing zoom modified the map pixels")
	}
}

func newStudioTest(t testing.TB) *FyneGUI {
	t.Helper()
	a := test.NewApp()
	a.Settings().SetTheme(studioTheme{theme.LightTheme()})
	g := &FyneGUI{app: a, opts: DefaultRenderOptions(), buildOpts: DefaultBuildOptions(), loading: true}
	g.window = a.NewWindow("MapArt Studio")
	g.setupUI()
	g.writeOptions(g.opts)
	g.matchPreset(g.opts)
	g.window.Resize(fyne.NewSize(1320, 850))
	t.Cleanup(func() { g.closed.Store(true); g.window.Close(); a.Quit() })
	return g
}

func TestStudioReducedMapPreviewKeepsColourMixtures(t *testing.T) {
	g := newStudioTest(t)
	source := image.NewRGBA(image.Rect(0, 0, 640, 384))
	g.showImage(g.resultPane, source)
	g.resultPane.content.Resize(fyne.NewSize(400, 300))
	g.resultPane.scroll.Refresh()
	if g.resultPane.image.ScaleMode != canvas.ImageScaleSmooth {
		t.Fatal("reduced map still drops dithered blocks")
	}
	g.zoom.SetChecked(true)
	if g.resultPane.image.ScaleMode != canvas.ImageScalePixels {
		t.Fatal("1:1 no longer shows exact map pixels")
	}
	if g.resultPane.image.Image != source {
		t.Fatal("preview resampling replaced export pixels")
	}
	g.zoom.SetChecked(false)
	g.resultPane.content.Resize(fyne.NewSize(800, 600))
	g.resultPane.scroll.Refresh()
	if g.resultPane.image.ScaleMode != canvas.ImageScalePixels {
		t.Fatal("enlarged map lost sharp blocks")
	}
}

func TestStudioNaturalBlocksProfileSelection(t *testing.T) {
	g := newStudioTest(t)
	opts := DefaultRenderOptions()
	opts.NaturalBlocks, opts.Terracotta = true, true
	g.writeOptions(opts)
	got, err := g.readOptions()
	if err != nil || got != opts {
		t.Fatal("additional materials lost through GUI", err, got)
	}
	var clay *widget.Check
	g.blockPicker.search.SetText("Глина")
	for _, object := range studioTestObjects(g.settings.Items[0].Content) {
		if row, ok := object.(*studioBlockRow); ok && row.title.Text == "Глина" {
			clay = row.check
		}
	}
	if clay == nil || !clay.Checked || !clay.Disabled() {
		t.Fatal("automatic palette does not show its clay block")
	}
	opts.SelectedBlocks = "minecraft:clay,minecraft:packed_ice"
	opts.BuildMode = "2d"
	g.writeOptions(opts)
	got, err = g.readOptions()
	if err != nil || got != opts || clay.Disabled() {
		t.Fatal("explicit additional block selection lost", got, err)
	}
	g.preset.SetSelected("landscape")
	g.applyPreset()
	got, err = g.readOptions()
	if err != nil || !got.NaturalBlocks || got.SelectedBlocks != opts.SelectedBlocks || got.BuildMode != "2d" {
		t.Fatal("landscape preset replaced selected blocks", got, err)
	}
}

func TestStudioExtendedCatalogSearchAndSelection(t *testing.T) {
	g := newStudioTest(t)
	opts := DefaultRenderOptions()
	opts.AllBlocks = true
	g.writeOptions(opts)
	got, err := g.readOptions()
	if err != nil || got != opts {
		t.Fatal("full catalogue changed through GUI", err)
	}
	var search *widget.Entry
	for _, object := range studioTestObjects(g.settings.Items[0].Content) {
		if field, ok := object.(*widget.Entry); ok && strings.HasPrefix(field.PlaceHolder, "Поиск:") {
			search = field
		}
	}
	if search == nil {
		t.Fatal("block search is missing")
	}
	search.SetText("гриб")
	visibleGroups := map[string]bool{}
	for _, id := range g.blockPicker.model.visible {
		visibleGroups[blockGroup(id)] = true
	}
	if len(visibleGroups) != 1 || !visibleGroups["Грибы и Незер"] {
		t.Fatal("search did not filter the catalogue", visibleGroups)
	}
	got, err = g.readOptions()
	if err != nil || got != opts {
		t.Fatal("search changed selected blocks", err)
	}
	test.Tap(findStudioTestButton(g.settings.Items[0].Content, "Терракота"))
	got, err = g.readOptions()
	if err != nil {
		t.Fatal(err)
	}
	ids, err := selectedBlockIDs(got.SelectedBlocks)
	if err != nil || len(ids) != 16 {
		t.Fatal("terracotta selection is incomplete", len(ids), err)
	}
	for id := range ids {
		if !strings.Contains(blockSelectionName(id), "terracotta") {
			t.Fatal("terracotta button left another material enabled")
		}
	}
	g.blockPicker.list.ScrollToBottom()
	search.SetText("pale_oak")
	if g.blockPicker.list.GetScrollOffset() != 0 {
		t.Fatal("search did not return the list to its first result")
	}
	afterSearch, err := g.readOptions()
	if err != nil || afterSearch != got {
		t.Fatal("hidden selections were lost", err)
	}
	if g.window.Canvas().Size().Width > 1320 {
		t.Fatal("catalogue widened the window")
	}
	if dir := os.Getenv("MAPART_GUI_SCREENSHOT_DIR"); dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := savePNG(filepath.Join(dir, "block-catalog.png"), g.window.Canvas().Capture()); err != nil {
			t.Fatal(err)
		}
	}
}

func TestStudioProfileAndPreset(t *testing.T) {
	g := newStudioTest(t)
	opts := DefaultRenderOptions()
	opts.MapsX, opts.MapsY, opts.Fit, opts.Resample = 2, 4, "contain", "nearest"
	opts.Crop, opts.Matte = "1,2,30,40", "#123456"
	opts.PaletteFile, opts.NoDitherMask, opts.DetailMask = "/tmp/palette.json", "/tmp/no.png", "/tmp/details.png"
	opts.Background, opts.BackgroundColor, opts.BackgroundTolerance = "color", "#abcdef", 60
	opts.Dither, opts.DitherStrength, opts.Threshold, opts.ColorMetric = "ordered", .78, 12.34, "rgb"
	opts.Brightness, opts.Contrast, opts.Saturation, opts.Gamma = 12.3, 1.23, 1.45, 1.56
	opts.Temperature, opts.Sharpen, opts.Denoise = -12.3, .43, .32
	opts.AutoContrast, opts.CleanHaze, opts.SkyCleanup, opts.Monochrome = true, true, false, true
	g.writeOptions(opts)
	got, err := g.readOptions()
	if err != nil || got != opts {
		t.Fatalf("profile settings changed through controls: err=%v\ngot=%+v\nwant=%+v", err, got, opts)
	}
	// Selecting a preset shows its description; Apply performs the change.
	g.preset.SetSelected("portrait")
	got, err = g.readOptions()
	if err != nil || got != opts {
		t.Fatal("selecting a preset applied it prematurely")
	}
	g.applyPreset()
	got, err = g.readOptions()
	portrait, _ := PresetOptions("portrait")
	want := preservePresetAssets(portrait, opts)
	if err != nil || got != want {
		t.Fatalf("preset failed to retain framing/assets: %+v, %v", got, err)
	}
	// A profile can be saved and loaded with these same settings.
	path := filepath.Join(t.TempDir(), "profile.json")
	if err := SaveOptions(path, got); err != nil {
		t.Fatal(err)
	}
	reloaded, err := LoadOptions(path, DefaultRenderOptions())
	if err != nil || reloaded != got {
		t.Fatalf("profile round trip: %v", err)
	}
}

func TestStudioRejectsInvalidNumericInput(t *testing.T) {
	g := newStudioTest(t)
	field := g.number("Тест", func() float64 { return 3 }, func(float64) {}, 1, 256, 0, false)
	var entry *widget.Entry
	for _, object := range field.(*widget.Form).Items[0].Widget.(*fyne.Container).Objects {
		if value, ok := object.(*widget.Entry); ok {
			entry = value
		}
	}
	if entry == nil {
		t.Fatal("numeric input is missing")
	}
	for _, bad := range []string{"", "abc", "NaN", "Inf", "2.5", "0", "257"} {
		entry.SetText(bad)
		if _, err := g.readOptions(); err == nil {
			t.Errorf("accepted invalid input %q", bad)
		}
	}
}

func TestStudioPreviewAndExportState(t *testing.T) {
	g := newStudioTest(t)
	opts := DefaultRenderOptions()
	opts.MapsX, opts.MapsY, opts.Dither = 1, 1, "none"
	g.writeOptions(opts)
	source := image.NewRGBA(image.Rect(0, 0, 16, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			source.SetRGBA(x, y, color.RGBA{uint8(x * 16), uint8(y * 16), 180, 255})
		}
	}
	result, err := RenderPreview(source, opts)
	if err != nil {
		t.Fatal(err)
	}
	build, err := BuildStructure(result, DefaultBuildOptions())
	if err != nil {
		t.Fatal(err)
	}
	g.showResult(result, opts, "sample.png", build, nil)
	g.updateButtons()
	if g.pngButton.Disabled() || g.nbtButton.Disabled() || g.zipButton.Disabled() {
		t.Fatal("completed render cannot be exported")
	}
	if g.mapPane.image.Image != build.MapImage || !strings.Contains(g.materials.Text, "РАЗМЕЩЕНИЕ") {
		t.Fatal("map preview or material instructions are missing")
	}
	mask := g.maskPane.image.Image.(*image.Gray)
	for i, on := range result.NoDitherMask {
		if (mask.Pix[i] == 255) != on {
			t.Fatalf("wrong mask at %d", i)
		}
	}
	g.zoom.SetChecked(true)
	for _, pane := range g.panes {
		if pane.image.FillMode != canvas.ImageFillOriginal || pane.scroll.Direction != container.ScrollBoth {
			t.Fatal("1:1 view does not scroll at original dimensions")
		}
	}
	g.zoom.SetChecked(false)
	if dir := os.Getenv("MAPART_GUI_SCREENSHOT_DIR"); dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		for i := range g.previewTabs.Items {
			g.previewTabs.SelectIndex(i)
			if err := savePNG(filepath.Join(dir, fmt.Sprintf("studio-tab-%d.png", i)), g.window.Canvas().Capture()); err != nil {
				t.Fatal(err)
			}
		}
		g.previewTabs.SelectIndex(0)
	}
	g.path.SetText("another.png")
	if !g.pngButton.Disabled() || !g.nbtButton.Disabled() {
		t.Fatal("stale preview can be exported after changing the source")
	}
	if g.resultPane.image.Image != result.Image {
		t.Fatal("editing settings removed the comparison preview")
	}
	g.showResult(result, opts, "sample.png", nil, fmt.Errorf("height limit"))
	g.updateButtons()
	if g.pngButton.Disabled() || g.mapPane.image.Image != nil {
		t.Fatal("build failure prevented PNG or left an old map preview")
	}
	opts.PaletteFile = "custom.json"
	g.showResult(result, opts, "sample.png", nil, fmt.Errorf("custom palette"))
	g.updateButtons()
	if g.pngButton.Disabled() || !g.nbtButton.Disabled() || !g.zipButton.Disabled() {
		t.Fatal("custom RGB palette should allow PNG and disable block exports")
	}
}

// Exercise the real dialog callbacks: constructing the studio does not open them.
func TestStudioFileDialogs(t *testing.T) {
	for _, name := range []string{"image", "profile"} {
		t.Run(name, func(t *testing.T) {
			g := newStudioTest(t)
			if name == "image" {
				g.openImageDialog()
			} else {
				g.loadProfile()
			}
			if len(g.window.Canvas().Overlays().List()) == 0 {
				t.Fatal("file chooser was not shown")
			}
		})
	}
}

func findStudioTestButton(object fyne.CanvasObject, text string) *widget.Button {
	if button, ok := object.(*widget.Button); ok && button.Text == text {
		return button
	}
	var children []fyne.CanvasObject
	switch value := object.(type) {
	case *fyne.Container:
		children = value.Objects
	case fyne.Widget:
		children = test.WidgetRenderer(value).Objects()
	}
	for _, child := range children {
		if button := findStudioTestButton(child, text); button != nil {
			return button
		}
	}
	return nil
}

func TestStudioSaveFolderDialog(t *testing.T) {
	g := newStudioTest(t)
	g.saveDialog("Сохранить превью", ".png", func(string) {
		t.Fatal("opening a folder chooser must not save a file")
	})
	overlays := g.window.Canvas().Overlays().List()
	browse := findStudioTestButton(overlays[len(overlays)-1], "Выбрать папку…")
	if browse == nil {
		t.Fatal("folder chooser button is missing")
	}
	test.Tap(browse)
	if len(g.window.Canvas().Overlays().List()) < 2 {
		t.Fatal("folder chooser was not shown above the save dialog")
	}
}

func studioTestObjects(object fyne.CanvasObject) []fyne.CanvasObject {
	objects := []fyne.CanvasObject{object}
	var children []fyne.CanvasObject
	switch v := object.(type) {
	case *fyne.Container:
		children = v.Objects
	case fyne.Widget:
		children = test.WidgetRenderer(v).Objects()
	}
	for _, child := range children {
		objects = append(objects, studioTestObjects(child)...)
	}
	return objects
}

func TestStudioBlockChoiceAndBuildMode(t *testing.T) {
	g := newStudioTest(t)
	opts := DefaultRenderOptions()
	opts.SelectedBlocks = "minecraft:white_wool,minecraft:black_terracotta"
	opts.BuildMode = "2d"
	opts.Rotation = 270
	opts.FlipHorizontal = true
	g.writeOptions(opts)
	got, err := g.readOptions()
	if err != nil || got != opts {
		t.Fatalf("new settings changed through UI: %+v %v", got, err)
	}
	g.preset.SetSelected("portrait")
	g.applyPreset()
	got, err = g.readOptions()
	if err != nil || got.BuildMode != "2d" || got.SelectedBlocks != opts.SelectedBlocks || got.Rotation != 270 || !got.FlipHorizontal {
		t.Fatal("preset reset selected blocks or source geometry")
	}
	opts.BuildMode = "3d"
	g.writeOptions(opts)
	got, err = g.readOptions()
	if err != nil || got != opts {
		t.Fatal("explicit 3d profile was changed")
	}
	clear := findStudioTestButton(g.settings.Items[0].Content, "Снять выбор")
	if clear == nil {
		t.Fatal("clear blocks action is missing")
	}
	test.Tap(clear)
	if _, err = g.readOptions(); err == nil {
		t.Fatal("UI allowed an empty block palette")
	}
	all := findStudioTestButton(g.settings.Items[0].Content, "Все блоки")
	test.Tap(all)
	got, err = g.readOptions()
	if err != nil {
		t.Fatal(err)
	}
	ids, err := selectedBlockIDs(got.SelectedBlocks)
	if err != nil || len(ids) != paletteBlockCount {
		t.Fatal("select all did not include all blocks")
	}
	var auto *widget.Check
	for _, object := range studioTestObjects(g.settings.Items[0].Content) {
		if c, ok := object.(*widget.Check); ok && strings.HasPrefix(c.Text, "Автоматическая палитра") {
			auto = c
			break
		}
	}
	if auto == nil {
		t.Fatal("automatic palette checkbox missing")
	}
	auto.SetChecked(true)
	g.updateButtons()
	for _, object := range studioTestObjects(g.settings.Items[0].Content) {
		if c, ok := object.(*widget.Check); ok && c != auto && !c.Disabled() {
			t.Fatal("automatic palette left per-block controls enabled")
		}
	}
}

func TestStudioSourceEditorCropAndCancel(t *testing.T) {
	g := newStudioTest(t)
	source := image.NewRGBA(image.Rect(0, 0, 20, 10))
	for y := 0; y < 10; y++ {
		for x := 0; x < 20; x++ {
			source.SetRGBA(x, y, color.RGBA{uint8(x * 12), uint8(y * 25), 160, 255})
		}
	}
	opts := g.opts
	g.sourceEditor(source, opts)
	overlays := g.window.Canvas().Overlays().List()
	overlay := overlays[len(overlays)-1]
	var edit *studioEditCanvas
	for _, object := range studioTestObjects(overlay) {
		if c, ok := object.(*studioEditCanvas); ok {
			edit = c
			break
		}
	}
	if edit == nil {
		t.Fatal("interactive source crop missing")
	}
	originalSize := edit.Size()
	edit.Resize(fyne.NewSize(400, 400))
	edit.MouseDown(&desktop.MouseEvent{PointEvent: fyne.PointEvent{Position: fyne.NewPos(40, 140)}, Button: desktop.MouseButtonPrimary})
	edit.Dragged(&fyne.DragEvent{PointEvent: fyne.PointEvent{Position: fyne.NewPos(240, 260)}})
	edit.DragEnd()
	if edit.crop != image.Rect(2, 2, 13, 9) {
		t.Fatal("crop did not follow drag", edit.crop)
	}
	test.Tap(findStudioTestButton(overlay, "↷ 90°"))
	if edit.src.Bounds().Size() != image.Pt(10, 20) || !edit.crop.Empty() {
		t.Fatal("rotation failed to swap dimensions and reset crop")
	}
	edit.Resize(originalSize)
	if dir := os.Getenv("MAPART_GUI_SCREENSHOT_DIR"); dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := savePNG(filepath.Join(dir, "source-editor.png"), g.window.Canvas().Capture()); err != nil {
			t.Fatal(err)
		}
	}
	test.Tap(findStudioTestButton(overlay, "Отмена"))
	if g.opts != opts {
		t.Fatal("cancelled source editor modified settings")
	}
}

func TestStudioMapEditorPaintingAndCancel(t *testing.T) {
	g := newStudioTest(t)
	opts := DefaultRenderOptions()
	opts.MapsX, opts.MapsY = 1, 1
	opts.BuildMode = "2d"
	opts.SelectedBlocks = "minecraft:white_wool,minecraft:black_wool"
	g.writeOptions(opts)
	source := image.NewRGBA(image.Rect(0, 0, 16, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			source.SetRGBA(x, y, color.RGBA{255, 255, 255, 255})
		}
	}
	result, err := RenderPreview(source, opts)
	if err != nil {
		t.Fatal(err)
	}
	s, err := BuildStructure(result, DefaultBuildOptions())
	if err != nil {
		t.Fatal(err)
	}
	g.showResult(result, opts, "sample.png", s, nil)
	g.updateButtons()
	if g.editorButton.Disabled() {
		t.Fatal("ready image cannot be edited")
	}
	g.editMap()
	overlays := g.window.Canvas().Overlays().List()
	overlay := overlays[len(overlays)-1]
	var edit *studioEditCanvas
	for _, object := range studioTestObjects(overlay) {
		if c, ok := object.(*studioEditCanvas); ok {
			edit = c
			break
		}
	}
	if edit == nil {
		t.Fatal("pixel editor canvas is missing")
	}
	// Painting uses the first (white) colour, so choose black through the palette.
	test.Tap(findStudioTestButton(overlay, "Выбрать цвет…"))
	overlays = g.window.Canvas().Overlays().List()
	paletteDialog := overlays[len(overlays)-1]
	button := findStudioTestButton(paletteDialog, "Чёрная шерсть · обычный")
	if button == nil {
		t.Fatal("selected block colour missing")
	}
	test.Tap(button)
	originalSize := edit.Size()
	edit.Resize(fyne.NewSize(256, 256))
	edit.MouseDown(&desktop.MouseEvent{PointEvent: fyne.PointEvent{Position: fyne.NewPos(20, 20)}, Button: desktop.MouseButtonPrimary})
	edit.Dragged(&fyne.DragEvent{PointEvent: fyne.PointEvent{Position: fyne.NewPos(40, 20)}})
	edit.DragEnd()
	if edit.src.(*image.RGBA).RGBAAt(15, 10) != palettePixel(result.Palette[1]) {
		t.Fatal("mouse stroke failed to paint continuous map pixels")
	}
	if result.Image.RGBAAt(15, 10) == palettePixel(result.Palette[1]) {
		t.Fatal("unapplied editor changed exported result")
	}
	test.Tap(findStudioTestButton(overlay, "Отменить"))
	if edit.src.(*image.RGBA).RGBAAt(15, 10) != palettePixel(result.Palette[0]) {
		t.Fatal("editor undo failed")
	}
	test.Tap(findStudioTestButton(overlay, "Вернуть"))
	if edit.src.(*image.RGBA).RGBAAt(15, 10) != palettePixel(result.Palette[1]) {
		t.Fatal("editor redo failed")
	}
	edit.Resize(originalSize)
	if dir := os.Getenv("MAPART_GUI_SCREENSHOT_DIR"); dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := savePNG(filepath.Join(dir, "map-editor.png"), g.window.Canvas().Capture()); err != nil {
			t.Fatal(err)
		}
	}
	test.Tap(findStudioTestButton(overlay, "Отмена"))
	if result.Image.RGBAAt(15, 10) != palettePixel(result.Palette[0]) {
		t.Fatal("cancelled editor leaked a manual edit")
	}
	g.changed()
	if !g.editorButton.Disabled() {
		t.Fatal("stale map was editable")
	}
}
