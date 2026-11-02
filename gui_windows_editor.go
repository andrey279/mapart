//go:build windows

package main

import (
	"fmt"
	"image"
	"math"
	"strings"

	"github.com/lxn/walk"
	d "github.com/lxn/walk/declarative"
	"github.com/lxn/win"
)

func (a *nativeApp) modeChoice() d.Widget {
	values := []string{buildModeLabel("3d"), buildModeLabel("2d")}
	var field *walk.ComboBox
	a.readFields = append(a.readFields, func() {
		if field.CurrentIndex() == 1 {
			a.options.BuildMode = "2d"
		} else if a.options.BuildMode != "3d" {
			a.options.BuildMode = ""
		}
	})
	a.writeFields = append(a.writeFields, func() {
		index := 0
		if a.options.BuildMode == "2d" {
			index = 1
		}
		field.SetCurrentIndex(index)
	})
	return d.Composite{Layout: d.VBox{MarginsZero: true}, Children: []d.Widget{
		d.Label{Text: "Режим постройки"}, d.ComboBox{AssignTo: &field, Model: values, OnCurrentIndexChanged: a.changed},
	}}
}

type nativeEditSurface struct {
	widget *walk.CustomWidget
	src    image.Image
	bitmap *walk.Bitmap
	crop   image.Rectangle
	zoom   float64
}

// Consume wheel messages here so the enclosing ScrollView does not also
// scroll the image vertically during a zoom gesture.
type nativeZoomWidget struct {
	*walk.CustomWidget
	onWheel func(float64)
}

func (w *nativeZoomWidget) WndProc(hwnd win.HWND, msg uint32, wParam, lParam uintptr) uintptr {
	if msg == win.WM_MOUSEWHEEL {
		w.onWheel(float64(int16(win.HIWORD(uint32(wParam)))) / 120)
		return 0
	}
	return w.CustomWidget.WndProc(hwnd, msg, wParam, lParam)
}

func (s *nativeEditSurface) setImage(src, display image.Image) error {
	bitmap, err := walk.NewBitmapFromImageForDPI(display, s.widget.DPI())
	if err != nil {
		return err
	}
	if s.bitmap != nil {
		s.bitmap.Dispose()
	}
	s.src, s.bitmap = src, bitmap
	return s.widget.Invalidate()
}

func (s *nativeEditSurface) geometry() (walk.Rectangle, float64) {
	b := s.widget.ClientBoundsPixels()
	w, h := s.src.Bounds().Dx(), s.src.Bounds().Dy()
	left, top, scale := editorImageGeometry(s.src.Bounds(), float64(b.Width), float64(b.Height), s.zoom*float64(s.widget.DPI())/96)
	width, height := int(math.Round(float64(w)*scale)), int(math.Round(float64(h)*scale))
	return walk.Rectangle{X: int(math.Round(left)), Y: int(math.Round(top)), Width: width, Height: height}, scale
}

func (s *nativeEditSurface) point(x, y int, clamp bool) (image.Point, bool) {
	b, _ := s.geometry()
	return containedImagePoint(s.src.Bounds(), float64(b.Width), float64(b.Height), float64(x-b.X), float64(y-b.Y), clamp)
}

func (s *nativeEditSurface) declaration() d.Widget {
	return d.CustomWidget{AssignTo: &s.widget, MinSize: d.Size{Width: 200, Height: 200}, StretchFactor: 1, InvalidatesOnResize: true, PaintPixels: func(c *walk.Canvas, _ walk.Rectangle) error {
		brush, err := walk.NewSolidColorBrush(walk.RGB(235, 238, 242))
		if err != nil {
			return err
		}
		defer brush.Dispose()
		if err := c.FillRectanglePixels(brush, s.widget.ClientBoundsPixels()); err != nil {
			return err
		}
		if s.bitmap == nil || s.src == nil {
			return nil
		}
		bounds, scale := s.geometry()
		if err := c.DrawImageStretchedPixels(s.bitmap, bounds); err != nil {
			return err
		}
		if !s.crop.Empty() {
			pen, err := walk.NewCosmeticPen(walk.PenSolid, walk.RGB(0, 100, 220))
			if err != nil {
				return err
			}
			defer pen.Dispose()
			return c.DrawRectanglePixels(pen, walk.Rectangle{X: bounds.X + int(float64(s.crop.Min.X)*scale), Y: bounds.Y + int(float64(s.crop.Min.Y)*scale), Width: int(float64(s.crop.Dx()) * scale), Height: int(float64(s.crop.Dy()) * scale)})
		}
		return nil
	}}
}

func (a *nativeApp) editSource() {
	if a.busy {
		return
	}
	opts, err := a.readOptions()
	if err != nil {
		a.showError(err)
		return
	}
	path := strings.TrimSpace(a.path.Text())
	if path == "" {
		a.showError(fmt.Errorf("сначала выберите исходное изображение"))
		return
	}
	var src image.Image
	generated := a.generatedSource
	a.startWork("Открытие редактора исходника…", func() error { var err error; src, err = studioSourceImage(path, generated); return err }, func() { a.sourceEditor(src, opts) })
}

func (a *nativeApp) sourceEditor(source image.Image, opts RenderOptions) {
	var dlg *walk.Dialog
	var apply, cancel *walk.PushButton
	var info *walk.Label
	surface := &nativeEditSurface{}
	refresh := func(resetCrop bool) {
		if surface.widget == nil {
			return
		}
		if resetCrop {
			opts.Crop = ""
			surface.crop = image.Rectangle{}
		}
		transformed := transformSource(source, opts)
		if err := surface.setImage(transformed, sourceTonePreview(transformed, opts)); err != nil {
			a.showError(err)
		}
		info.SetText("Выделите мышью область кадрирования. Обрезка: " + opts.Crop)
	}
	tone := func(label string, value *float64, low, high float64, decimals int) d.Widget {
		var field *walk.NumberEdit
		return d.Composite{Layout: d.HBox{MarginsZero: true}, Children: []d.Widget{d.Label{Text: label, MinSize: d.Size{Width: 140, Height: 0}}, d.NumberEdit{AssignTo: &field, Value: *value, MinValue: low, MaxValue: high, Decimals: decimals, SpinButtonsVisible: true, OnValueChanged: func() {
			if field != nil {
				*value = field.Value()
				refresh(false)
			}
		}}}}
	}
	definition := d.Dialog{AssignTo: &dlg, Title: "Редактор исходника", Size: d.Size{Width: 1000, Height: 800}, MinSize: d.Size{Width: 700, Height: 600}, Layout: d.VBox{}, DefaultButton: &apply, CancelButton: &cancel, Children: []d.Widget{
		d.Composite{Layout: d.HBox{MarginsZero: true}, Children: []d.Widget{
			d.PushButton{Text: "↶ 90°", OnClicked: func() { rotateSourceOptions(&opts, -90); refresh(true) }},
			d.PushButton{Text: "↷ 90°", OnClicked: func() { rotateSourceOptions(&opts, 90); refresh(true) }},
			d.PushButton{Text: "Отразить ↔", OnClicked: func() { opts.FlipHorizontal = !opts.FlipHorizontal; refresh(true) }},
			d.PushButton{Text: "Отразить ↕", OnClicked: func() { opts.FlipVertical = !opts.FlipVertical; refresh(true) }},
			d.PushButton{Text: "Сбросить обрезку", OnClicked: func() { refresh(true) }},
		}},
		d.Label{AssignTo: &info}, surface.declaration(),
		tone("Яркость", &opts.Brightness, -100, 100, 1), tone("Контраст", &opts.Contrast, .1, 3, 2),
		tone("Насыщенность", &opts.Saturation, 0, 3, 2), tone("Гамма", &opts.Gamma, .2, 3, 2), tone("Температура", &opts.Temperature, -100, 100, 1),
		d.Composite{Layout: d.HBox{}, Children: []d.Widget{d.HSpacer{}, d.PushButton{AssignTo: &apply, Text: "Применить и преобразовать", OnClicked: func() { dlg.Accept() }}, d.PushButton{AssignTo: &cancel, Text: "Отмена", OnClicked: func() { dlg.Cancel() }}}},
	}}
	if err := definition.Create(a.window); err != nil {
		a.showError(err)
		return
	}
	defer dlg.Dispose()
	defer func() {
		if surface.bitmap != nil {
			surface.bitmap.Dispose()
		}
	}()
	surface.crop, _ = ParseCrop(opts.Crop)
	refresh(false)
	var start image.Point
	active := false
	selectCrop := func(x, y int) {
		p, _ := surface.point(x, y, true)
		surface.crop = image.Rect(min(start.X, p.X), min(start.Y, p.Y), max(start.X, p.X)+1, max(start.Y, p.Y)+1)
		opts.Crop = fmt.Sprintf("%d,%d,%d,%d", surface.crop.Min.X, surface.crop.Min.Y, surface.crop.Dx(), surface.crop.Dy())
		info.SetText("Обрезка: " + opts.Crop)
		surface.widget.Invalidate()
	}
	surface.widget.MouseDown().Attach(func(x, y int, button walk.MouseButton) {
		if button != walk.LeftButton {
			return
		}
		p, ok := surface.point(x, y, false)
		if !ok {
			return
		}
		start = p
		active = true
		win.SetCapture(surface.widget.Handle())
		selectCrop(x, y)
	})
	surface.widget.MouseMove().Attach(func(x, y int, _ walk.MouseButton) {
		if active {
			selectCrop(x, y)
		}
	})
	surface.widget.MouseUp().Attach(func(x, y int, _ walk.MouseButton) {
		if active {
			selectCrop(x, y)
			active = false
			win.ReleaseCapture()
		}
	})
	if dlg.Run() == walk.DlgCmdOK {
		a.writeOptions(opts)
		a.render()
	}
}

func (a *nativeApp) editMap() {
	if a.busy || a.result == nil || a.dirty {
		return
	}
	result := a.result
	editor := NewPixelEditor(result.Image)
	var dlg *walk.Dialog
	var apply, cancel *walk.PushButton
	var tool, brush, colour, zoom *walk.ComboBox
	var zoomPercent *walk.Label
	surface := &nativeEditSurface{}
	active, lastValid := false, false
	currentScale := func() float64 {
		_, scale := surface.geometry()
		return scale * 96 / float64(surface.widget.DPI())
	}
	updateZoomLabel := func() {
		if surface.widget != nil && surface.src != nil && zoomPercent != nil {
			zoomPercent.SetText(fmt.Sprintf("%.1f%%", currentScale()*100))
		}
	}
	setZoom := func(value float64) {
		if surface.widget == nil || active {
			return
		}
		surface.zoom = 0
		size := walk.Size{Width: 200, Height: 200}
		if value > 0 {
			surface.zoom = max(minEditorZoom, min(maxEditorZoom, value))
			size = walk.Size{Width: max(1, int(math.Round(float64(editor.Image.Bounds().Dx())*surface.zoom))), Height: max(1, int(math.Round(float64(editor.Image.Bounds().Dy())*surface.zoom)))}
		}
		surface.widget.SetMinMaxSize(size, walk.Size{})
		surface.widget.Invalidate()
		updateZoomLabel()
	}
	stepZoom := func(direction float64) {
		if surface.widget == nil || surface.src == nil || active || direction == 0 {
			return
		}
		setZoom(editorZoomStep(currentScale(), direction))
		zoom.SetCurrentIndex(-1)
	}
	refresh := func() {
		if surface.widget != nil {
			if err := surface.setImage(editor.Image, editor.Image); err != nil {
				a.showError(err)
			}
		}
	}
	labels := make([]string, len(result.Palette))
	for i, p := range result.Palette {
		pixel := palettePixel(p)
		labels[i] = fmt.Sprintf("#%02X%02X%02X", pixel.R, pixel.G, pixel.B)
		if p.Shade != "Custom" {
			labels[i] = blockLabel(p.WoolID) + " · " + map[string]string{"Dark": "тёмный", "Normal": "обычный", "Light": "светлый"}[p.Shade] + " " + labels[i]
		}
	}
	definition := d.Dialog{AssignTo: &dlg, Title: "Редактор карты — один пиксель = один блок", Size: d.Size{Width: 1100, Height: 800}, MinSize: d.Size{Width: 800, Height: 500}, Layout: d.VBox{}, DefaultButton: &apply, CancelButton: &cancel, Children: []d.Widget{
		d.Composite{Layout: d.HBox{MarginsZero: true}, Children: []d.Widget{
			d.ComboBox{AssignTo: &tool, Model: []string{"Кисть", "Заливка", "Пипетка"}, CurrentIndex: 0},
			d.Label{Text: "Размер кисти"}, d.ComboBox{AssignTo: &brush, Model: []string{"1", "3", "5", "9", "17"}, CurrentIndex: 0},
			d.PushButton{Text: "Отменить", OnClicked: func() { editor.Undo(); refresh() }}, d.PushButton{Text: "Вернуть", OnClicked: func() { editor.Redo(); refresh() }},
		}},
		d.ComboBox{AssignTo: &colour, Model: labels, CurrentIndex: 0},
		d.Composite{Layout: d.HBox{MarginsZero: true}, Children: []d.Widget{
			d.PushButton{Text: "−", OnClicked: func() { stepZoom(-1) }},
			d.PushButton{Text: "+", OnClicked: func() { stepZoom(1) }},
			d.ComboBox{AssignTo: &zoom, Model: editorZoomChoices, CurrentIndex: 0, OnCurrentIndexChanged: func() {
				if zoom != nil && zoom.CurrentIndex() >= 0 {
					setZoom(editorZoomChoice(zoom.Text()))
				}
			}},
			d.Label{AssignTo: &zoomPercent, MinSize: d.Size{Width: 65, Height: 0}},
			d.PushButton{Text: "Вписать", OnClicked: func() { zoom.SetCurrentIndex(0); setZoom(0) }},
			d.Label{Text: "Колёсико мыши — приблизить / отдалить"}, d.HSpacer{},
		}},
		d.ScrollView{Layout: d.VBox{MarginsZero: true}, StretchFactor: 1, Children: []d.Widget{surface.declaration()}},
		d.TextLabel{Text: "Примените правки, чтобы сохранить их в PNG и NBT. Повторное преобразование исходника заменяет ручные правки."},
		d.Composite{Layout: d.HBox{}, Children: []d.Widget{d.HSpacer{}, d.PushButton{AssignTo: &apply, Text: "Применить правки", OnClicked: func() { dlg.Accept() }}, d.PushButton{AssignTo: &cancel, Text: "Отмена", OnClicked: func() { dlg.Cancel() }}}},
	}}
	if err := definition.Create(a.window); err != nil {
		a.showError(err)
		return
	}
	defer dlg.Dispose()
	defer func() {
		if surface.bitmap != nil {
			surface.bitmap.Dispose()
		}
	}()
	refresh()
	updateZoomLabel()
	surface.widget.SizeChanged().Attach(updateZoomLabel)
	if err := walk.InitWrapperWindow(&nativeZoomWidget{CustomWidget: surface.widget, onWheel: stepZoom}); err != nil {
		a.showError(err)
		return
	}
	var last image.Point
	paint := func(x, y int, drag bool) {
		p, ok := surface.point(x, y, false)
		if !ok {
			lastValid = false
			return
		}
		if tool.CurrentIndex() == 2 {
			c := editor.Image.RGBAAt(p.X, p.Y)
			for i, entry := range result.Palette {
				if palettePixel(entry) == c {
					colour.SetCurrentIndex(i)
					break
				}
			}
			return
		}
		selected := palettePixel(result.Palette[max(0, colour.CurrentIndex())])
		if tool.CurrentIndex() == 1 {
			if !drag {
				editor.Fill(p, selected)
				editor.Commit()
				refresh()
			}
			return
		}
		var size int
		fmt.Sscan(brush.Text(), &size)
		if drag && lastValid {
			editor.Line(last, p, size/2, selected)
		} else {
			editor.Paint(p, size/2, selected)
		}
		last, lastValid = p, true
		refresh()
	}
	surface.widget.MouseDown().Attach(func(x, y int, button walk.MouseButton) {
		if button != walk.LeftButton {
			return
		}
		active, lastValid = true, false
		win.SetCapture(surface.widget.Handle())
		paint(x, y, false)
	})
	surface.widget.MouseMove().Attach(func(x, y int, _ walk.MouseButton) {
		if active {
			paint(x, y, true)
		}
	})
	surface.widget.MouseUp().Attach(func(x, y int, _ walk.MouseButton) {
		if active {
			paint(x, y, true)
			active = false
			editor.Commit()
			win.ReleaseCapture()
		}
	})
	if dlg.Run() != walk.DlgCmdOK {
		return
	}
	editor.Commit()
	updated := *result
	updated.Image = cloneRGBA(editor.Image)
	build := a.buildOptions()
	var structure *Structure
	var buildErr error
	a.startWork("Обновление схемы после правок…", func() error { structure, buildErr = BuildStructure(&updated, build); return nil }, func() {
		a.result, a.structure = &updated, structure
		if err := a.showImage(a.previewView, updated.Image); err != nil {
			a.showError(err)
			return
		}
		if buildErr != nil {
			a.clearImage(a.mapView)
			a.buildText.SetText(buildErr.Error())
			a.status.SetText("Правки применены. Схема: " + buildErr.Error())
		} else {
			a.showBuild(structure)
			a.status.SetText("Правки применены. Можно сохранить PNG и NBT.")
		}
		if a.resultSource == generatedQRSource && a.qrOptions.Text != "" && buildErr == nil {
			a.status.SetText(qrReadabilityStatus(&updated, structure, a.qrOptions.Text))
		}
		a.updateButtons()
	})
}
