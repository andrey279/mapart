//go:build !windows && gui

package main

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

func (g *FyneGUI) modeChoice() fyne.CanvasObject {
	values := []string{buildModeLabel("3d"), buildModeLabel("2d")}
	field := widget.NewSelect(values, func(string) { g.changed() })
	g.track(field)
	g.readFields = append(g.readFields, func() error {
		if field.Selected == values[1] {
			g.opts.BuildMode = "2d"
		} else if g.opts.BuildMode != "3d" {
			g.opts.BuildMode = ""
		}
		return nil
	})
	g.writeFields = append(g.writeFields, func() { field.SetSelected(buildModeLabel(g.opts.BuildMode)) })
	return widget.NewForm(widget.NewFormItem("Режим", field))
}

// A shared interactive surface for selecting a source crop and painting maps.
type studioEditCanvas struct {
	widget.BaseWidget
	picture                     *canvas.Image
	background, selection       *canvas.Rectangle
	src                         image.Image
	display                     image.Image
	zoom                        float32
	crop                        image.Rectangle
	cropping, active, lastValid bool
	start, last                 image.Point
	onPoint                     func(image.Point, bool)
	onEnd                       func()
	onCrop                      func(image.Rectangle)
	onScroll                    func(*fyne.ScrollEvent)
	onLayout                    func()
}

func newStudioEditCanvas(src image.Image, cropping bool) *studioEditCanvas {
	c := &studioEditCanvas{src: src, cropping: cropping}
	c.picture = canvas.NewImageFromImage(src)
	c.picture.FillMode = canvas.ImageFillContain
	c.picture.ScaleMode = canvas.ImageScalePixels
	c.background = canvas.NewRectangle(color.NRGBA{235, 238, 242, 255})
	c.selection = canvas.NewRectangle(color.NRGBA{0, 100, 220, 45})
	c.selection.StrokeColor = color.NRGBA{0, 100, 220, 255}
	c.selection.StrokeWidth = 2
	c.selection.Hide()
	c.ExtendBaseWidget(c)
	return c
}

func (c *studioEditCanvas) CreateRenderer() fyne.WidgetRenderer {
	return &studioEditRenderer{c: c}
}

type studioEditRenderer struct{ c *studioEditCanvas }

func (r *studioEditRenderer) Destroy() {}
func (r *studioEditRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.c.background, r.c.picture, r.c.selection}
}
func (r *studioEditRenderer) MinSize() fyne.Size {
	if r.c.zoom > 0 {
		return fyne.NewSize(float32(r.c.src.Bounds().Dx())*r.c.zoom, float32(r.c.src.Bounds().Dy())*r.c.zoom)
	}
	return fyne.NewSize(200, 200)
}
func (r *studioEditRenderer) Layout(size fyne.Size) {
	c := r.c
	c.background.Resize(size)
	b := c.src.Bounds()
	left, top, scale := editorImageGeometry(b, float64(size.Width), float64(size.Height), float64(c.zoom))
	c.picture.ScaleMode = canvas.ImageScalePixels
	if scale < 1 {
		c.picture.ScaleMode = canvas.ImageScaleSmooth
	}
	c.picture.Move(fyne.NewPos(float32(left), float32(top)))
	c.picture.Resize(fyne.NewSize(float32(float64(b.Dx())*scale), float32(float64(b.Dy())*scale)))
	if c.onLayout != nil {
		c.onLayout()
	}
	if c.crop.Empty() {
		c.selection.Hide()
		return
	}
	c.selection.Move(fyne.NewPos(float32(left+float64(c.crop.Min.X)*scale), float32(top+float64(c.crop.Min.Y)*scale)))
	c.selection.Resize(fyne.NewSize(float32(float64(c.crop.Dx())*scale), float32(float64(c.crop.Dy())*scale)))
	c.selection.Show()
}
func (r *studioEditRenderer) Refresh() {
	r.c.picture.Image = r.c.src
	if r.c.display != nil {
		r.c.picture.Image = r.c.display
	}
	r.Layout(r.c.Size())
	r.c.picture.Refresh()
	r.c.selection.Refresh()
}

func (c *studioEditCanvas) point(pos fyne.Position, clamp bool) (image.Point, bool) {
	left, top, scale := editorImageGeometry(c.src.Bounds(), float64(c.Size().Width), float64(c.Size().Height), float64(c.zoom))
	return containedImagePoint(c.src.Bounds(), float64(c.src.Bounds().Dx())*scale, float64(c.src.Bounds().Dy())*scale, float64(pos.X)-left, float64(pos.Y)-top, clamp)
}

func (c *studioEditCanvas) Scrolled(e *fyne.ScrollEvent) {
	if c.onScroll != nil && !c.active {
		c.onScroll(e)
	}
}

func studioMapZoom(view *studioEditCanvas) (*container.Scroll, fyne.CanvasObject) {
	scroll := container.NewScroll(view)
	scroll.Direction = container.ScrollNone
	percent := widget.NewLabel("")
	currentScale := func() float64 {
		_, _, scale := editorImageGeometry(view.src.Bounds(), float64(view.Size().Width), float64(view.Size().Height), float64(view.zoom))
		return scale
	}
	updateLabel := func() {
		if scale := currentScale(); scale > 0 {
			percent.SetText(fmt.Sprintf("%.1f%%", scale*100))
		}
	}
	view.onLayout = updateLabel
	setZoom := func(value float64, anchor fyne.Position) {
		if view.active {
			return
		}
		left, top, scale := editorImageGeometry(view.src.Bounds(), float64(view.Size().Width), float64(view.Size().Height), float64(view.zoom))
		if scale <= 0 {
			return
		}
		imageX, imageY := (float64(anchor.X)-left)/scale, (float64(anchor.Y)-top)/scale
		viewportX, viewportY := anchor.X-scroll.Offset.X, anchor.Y-scroll.Offset.Y
		view.zoom = 0
		scroll.Direction = container.ScrollNone
		if value > 0 {
			view.zoom = float32(max(minEditorZoom, min(maxEditorZoom, value)))
			scroll.Direction = container.ScrollBoth
		}
		view.Refresh()
		scroll.Refresh()
		if view.zoom == 0 {
			scroll.ScrollToOffset(fyne.Position{})
		} else {
			left, top, scale = editorImageGeometry(view.src.Bounds(), float64(view.Size().Width), float64(view.Size().Height), float64(view.zoom))
			scroll.ScrollToOffset(fyne.NewPos(float32(left+imageX*scale)-viewportX, float32(top+imageY*scale)-viewportY))
		}
		updateLabel()
	}
	centre := func() fyne.Position {
		return fyne.NewPos(scroll.Offset.X+scroll.Size().Width/2, scroll.Offset.Y+scroll.Size().Height/2)
	}
	zoom := widget.NewSelect(editorZoomChoices, nil)
	zoom.SetSelected("Вписать")
	zoom.OnChanged = func(value string) { setZoom(editorZoomChoice(value), centre()) }
	step := func(direction float64, anchor fyne.Position) {
		if view.active {
			return
		}
		setZoom(editorZoomStep(currentScale(), direction), anchor)
		// A wheel step need not coincide with one of the fixed choices.
		zoom.Selected = ""
		zoom.PlaceHolder = percent.Text
		zoom.Refresh()
	}
	view.onScroll = func(e *fyne.ScrollEvent) {
		if e.Scrolled.DY != 0 {
			step(math.Copysign(1, float64(e.Scrolled.DY)), e.Position)
		}
	}
	controls := container.NewHBox(
		widget.NewButton("−", func() { step(-1, centre()) }),
		widget.NewButton("+", func() { step(1, centre()) }),
		zoom, percent,
		widget.NewButton("Вписать", func() { zoom.SetSelected("Вписать"); setZoom(0, centre()) }),
		widget.NewLabel("Колёсико мыши — приблизить / отдалить"))
	return scroll, controls
}
func (c *studioEditCanvas) MouseDown(e *desktop.MouseEvent) {
	if e.Button != desktop.MouseButtonPrimary {
		return
	}
	p, ok := c.point(e.Position, false)
	if !ok {
		return
	}
	c.active, c.lastValid, c.start, c.last = true, false, p, p
	if c.cropping {
		c.selectCrop(p)
	} else if c.onPoint != nil {
		c.onPoint(p, false)
	}
}
func (c *studioEditCanvas) MouseUp(*desktop.MouseEvent) { c.DragEnd() }
func (c *studioEditCanvas) Dragged(e *fyne.DragEvent) {
	if !c.active {
		return
	}
	p, ok := c.point(e.Position, c.cropping)
	if !ok {
		c.lastValid = false
		return
	}
	if c.cropping {
		c.selectCrop(p)
	} else if c.onPoint != nil {
		c.onPoint(p, true)
	}
}
func (c *studioEditCanvas) DragEnd() {
	if !c.active {
		return
	}
	c.active = false
	if c.onEnd != nil {
		c.onEnd()
	}
}
func (c *studioEditCanvas) selectCrop(p image.Point) {
	c.crop = image.Rect(min(c.start.X, p.X), min(c.start.Y, p.Y), max(c.start.X, p.X)+1, max(c.start.Y, p.Y)+1)
	c.Refresh()
	if c.onCrop != nil {
		c.onCrop(c.crop)
	}
}

func (g *FyneGUI) editSource() {
	if g.busy {
		return
	}
	opts, err := g.readOptions()
	if err != nil {
		g.showError(err)
		return
	}
	path := strings.TrimSpace(g.path.Text)
	if path == "" {
		g.showError(fmt.Errorf("сначала выберите исходное изображение"))
		return
	}
	var source image.Image
	generated := g.generatedSource
	g.startWork("Открытие редактора исходника…", func() error {
		var err error
		source, err = studioSourceImage(path, generated)
		return err
	}, func() { g.sourceEditor(source, opts) })
}

func (g *FyneGUI) sourceEditor(source image.Image, opts RenderOptions) {
	view := newStudioEditCanvas(transformSource(source, opts), true)
	view.display = sourceTonePreview(view.src, opts)
	view.crop, _ = ParseCrop(opts.Crop)
	info := wrapped("")
	view.onCrop = func(rect image.Rectangle) {
		opts.Crop = fmt.Sprintf("%d,%d,%d,%d", rect.Min.X, rect.Min.Y, rect.Dx(), rect.Dy())
		info.SetText("Обрезка: " + opts.Crop)
	}
	refresh := func() {
		view.src = transformSource(source, opts)
		view.display = sourceTonePreview(view.src, opts)
		view.crop, opts.Crop = image.Rectangle{}, ""
		view.Refresh()
		info.SetText(fmt.Sprintf("Исходник: %d×%d. Выделите мышью область для кадрирования.", view.src.Bounds().Dx(), view.src.Bounds().Dy()))
	}
	info.SetText("Выделите мышью область для кадрирования. Обрезка: " + opts.Crop)
	tools := container.NewHBox(
		widget.NewButton("↶ 90°", func() { rotateSourceOptions(&opts, -90); refresh() }),
		widget.NewButton("↷ 90°", func() { rotateSourceOptions(&opts, 90); refresh() }),
		widget.NewButton("Отразить ↔", func() { opts.FlipHorizontal = !opts.FlipHorizontal; refresh() }),
		widget.NewButton("Отразить ↕", func() { opts.FlipVertical = !opts.FlipVertical; refresh() }),
		widget.NewButton("Сбросить обрезку", func() { refresh() }))
	tone := func(label string, value *float64, low, high, step float64) fyne.CanvasObject {
		slider := widget.NewSlider(low, high)
		slider.Step = step
		slider.SetValue(*value)
		amount := widget.NewLabel(fmt.Sprintf("%.2f", *value))
		slider.OnChanged = func(v float64) {
			*value = v
			amount.SetText(fmt.Sprintf("%.2f", v))
			view.display = sourceTonePreview(view.src, opts)
			view.Refresh()
		}
		return container.NewBorder(nil, nil, widget.NewLabel(label), amount, slider)
	}
	adjustments := container.NewVBox(
		wrapped("Цвет обновляется сразу. Выделенная область будет обрезана при преобразовании."),
		tone("Яркость", &opts.Brightness, -100, 100, 1),
		tone("Контраст", &opts.Contrast, .1, 3, .05),
		tone("Насыщенность", &opts.Saturation, 0, 3, .05),
		tone("Гамма", &opts.Gamma, .2, 3, .05),
		tone("Температура", &opts.Temperature, -100, 100, 1))
	content := container.NewBorder(container.NewVBox(tools, info), adjustments, nil, nil, view)
	fd := dialog.NewCustomConfirm("Редактор исходника", "Применить и преобразовать", "Отмена", content, func(ok bool) {
		if !ok {
			return
		}
		g.writeOptions(opts)
		g.convertImage()
	}, g.window)
	fd.Resize(fyne.NewSize(950, 760))
	fd.Show()
}

func (g *FyneGUI) editMap() {
	if g.busy || g.result == nil || g.dirty {
		return
	}
	result := g.result
	editor := NewPixelEditor(result.Image)
	view := newStudioEditCanvas(editor.Image, false)
	info := wrapped("Кисть и заливка используют только цвета выбранных блоков. Примените правки, чтобы сохранить их в PNG и NBT. Повторное преобразование исходника заменяет ручные правки.")
	tool := widget.NewSelect([]string{"Кисть", "Заливка", "Пипетка"}, nil)
	tool.SetSelected("Кисть")
	brush := widget.NewSelect([]string{"1", "3", "5", "9", "17"}, nil)
	brush.SetSelected("1")
	selected := palettePixel(result.Palette[0])
	swatch := canvas.NewRectangle(selected)
	swatch.SetMinSize(fyne.NewSize(36, 30))
	colorName := widget.NewLabel("")
	choose := func(index int) {
		p := result.Palette[index]
		selected = palettePixel(p)
		swatch.FillColor = selected
		swatch.Refresh()
		if p.Shade == "Custom" {
			colorName.SetText(fmt.Sprintf("#%02X%02X%02X", selected.R, selected.G, selected.B))
			return
		}
		shades := map[string]string{"Dark": "тёмный", "Normal": "обычный", "Light": "светлый"}
		colorName.SetText(blockLabel(p.WoolID) + " · " + shades[p.Shade])
	}
	choose(0)
	chooseColor := widget.NewButton("Выбрать цвет…", func() {
		var colorDialog *dialog.CustomDialog
		var objects []fyne.CanvasObject
		for i, p := range result.Palette {
			label := fmt.Sprintf("#%02X%02X%02X", palettePixel(p).R, palettePixel(p).G, palettePixel(p).B)
			if p.Shade != "Custom" {
				label = blockLabel(p.WoolID) + " · " + map[string]string{"Dark": "тёмный", "Normal": "обычный", "Light": "светлый"}[p.Shade]
			}
			chip := canvas.NewRectangle(palettePixel(p))
			chip.SetMinSize(fyne.NewSize(30, 28))
			button := widget.NewButton(label, func() { choose(i); colorDialog.Hide() })
			objects = append(objects, container.NewBorder(nil, nil, chip, nil, button))
		}
		colorDialog = dialog.NewCustom("Цвета карты", "Закрыть", container.NewVScroll(container.NewVBox(objects...)), g.window)
		colorDialog.Resize(fyne.NewSize(540, 600))
		colorDialog.Show()
	})
	view.onPoint = func(p image.Point, drag bool) {
		if tool.Selected == "Пипетка" {
			c := editor.Image.RGBAAt(p.X, p.Y)
			for i, entry := range result.Palette {
				if palettePixel(entry) == c {
					choose(i)
					break
				}
			}
			return
		}
		if tool.Selected == "Заливка" {
			if !drag {
				editor.Fill(p, selected)
				editor.Commit()
				view.Refresh()
			}
			return
		}
		var size int
		fmt.Sscan(brush.Selected, &size)
		if drag && view.lastValid {
			editor.Line(view.last, p, size/2, selected)
		} else {
			editor.Paint(p, size/2, selected)
		}
		view.last, view.lastValid = p, true
		view.Refresh()
	}
	view.onEnd = func() { editor.Commit(); view.lastValid = false }
	scroll, zoomControls := studioMapZoom(view)
	tools := container.NewHBox(tool, widget.NewLabel("Кисть"), brush,
		widget.NewButtonWithIcon("Отменить", theme.ContentUndoIcon(), func() { editor.Undo(); view.Refresh() }),
		widget.NewButtonWithIcon("Вернуть", theme.ContentRedoIcon(), func() { editor.Redo(); view.Refresh() }))
	colors := container.NewHBox(swatch, chooseColor, colorName)
	content := container.NewBorder(container.NewVBox(tools, colors, zoomControls), info, nil, nil, scroll)
	fd := dialog.NewCustomConfirm("Редактор карты — один пиксель = один блок", "Применить правки", "Отмена", content, func(ok bool) {
		if !ok {
			return
		}
		editor.Commit()
		build, err := g.readBuild()
		if err != nil {
			g.showError(err)
			return
		}
		updated := *result
		updated.Image = cloneRGBA(editor.Image)
		var structure *Structure
		var buildErr error
		g.startWork("Обновление схемы после правок…", func() error {
			structure, buildErr = BuildStructure(&updated, build)
			return nil
		}, func() { g.showResult(&updated, g.resultOpts, g.resultSource, structure, buildErr) })
	}, g.window)
	fd.Resize(fyne.NewSize(1050, 750))
	fd.Show()
}
