package main

import (
	"image"
	"image/color"
	"image/draw"
	"math"
	"strconv"
	"strings"

	xdraw "golang.org/x/image/draw"
)

const minEditorZoom = 1.0 / 256
const maxEditorZoom = 32.0

var editorZoomChoices = []string{"Вписать", "25%", "50%", "100%", "200%", "400%", "800%", "1600%", "3200%"}

func editorZoomChoice(value string) float64 {
	percent, _ := strconv.ParseFloat(strings.TrimSuffix(value, "%"), 64)
	return percent / 100
}

func editorZoomStep(current, direction float64) float64 {
	return max(minEditorZoom, min(maxEditorZoom, current*math.Pow(1.25, direction)))
}

// Explicit zoom sizes the image itself, even when it is smaller than the
// viewport; zooming out must not silently stretch it back to fill the editor.
func editorImageGeometry(bounds image.Rectangle, width, height, zoom float64) (left, top, scale float64) {
	if bounds.Empty() || width <= 0 || height <= 0 {
		return 0, 0, 0
	}
	scale = zoom
	if scale <= 0 {
		scale = min(width/float64(bounds.Dx()), height/float64(bounds.Dy()))
	}
	left, top = (width-float64(bounds.Dx())*scale)/2, (height-float64(bounds.Dy())*scale)/2
	return
}

// Bound live colour feedback so sliders remain responsive on large photos.
func sourceTonePreview(src image.Image, opts RenderOptions) *image.RGBA {
	b := src.Bounds()
	scale := min(1.0, 1000.0/float64(max(b.Dx(), b.Dy())))
	out := image.NewRGBA(image.Rect(0, 0, max(1, int(math.Round(float64(b.Dx())*scale))), max(1, int(math.Round(float64(b.Dy())*scale)))))
	matte, _ := ParseHexColor(opts.Matte)
	draw.Draw(out, out.Bounds(), image.NewUniform(matte), image.Point{}, draw.Src)
	xdraw.CatmullRom.Scale(out, out.Bounds(), src, b, draw.Over, nil)
	adjustImageTones(out, out.Bounds(), opts)
	return out
}

func transformedBounds(bounds image.Rectangle, rotation int) image.Rectangle {
	w, h := bounds.Dx(), bounds.Dy()
	if rotation == 90 || rotation == 270 {
		w, h = h, w
	}
	return image.Rect(0, 0, w, h)
}

// Rotate what the user currently sees, including any earlier reflection.
func rotateSourceOptions(opts *RenderOptions, degrees int) {
	if opts.FlipHorizontal != opts.FlipVertical {
		degrees = -degrees
	}
	opts.Rotation = (opts.Rotation + degrees + 360) % 360
}

// Rotation precedes mirroring and cropping. The same transform is used for
// source-sized masks, keeping all editing tools in the same coordinates.
func transformSource(src image.Image, opts RenderOptions) image.Image {
	if opts.Rotation == 0 && !opts.FlipHorizontal && !opts.FlipVertical {
		return src
	}
	b := src.Bounds()
	dst := image.NewNRGBA(transformedBounds(b, opts.Rotation))
	w, h := dst.Bounds().Dx(), dst.Bounds().Dy()
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			xx, yy := x, y
			switch opts.Rotation {
			case 90:
				xx, yy = b.Dy()-1-y, x
			case 180:
				xx, yy = b.Dx()-1-x, b.Dy()-1-y
			case 270:
				xx, yy = y, b.Dx()-1-x
			}
			if opts.FlipHorizontal {
				xx = w - 1 - xx
			}
			if opts.FlipVertical {
				yy = h - 1 - yy
			}
			dst.Set(xx, yy, src.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return dst
}

func cloneRGBA(src *image.RGBA) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, src.Bounds().Dx(), src.Bounds().Dy()))
	draw.Draw(dst, dst.Bounds(), src, src.Bounds().Min, draw.Src)
	return dst
}

type pixelChange struct {
	Point         image.Point
	Before, After color.RGBA
}

// History records changed pixels rather than copying a potentially large map
// for every brush stroke. A stroke (including a flood fill) is one undo action.
type PixelEditor struct {
	Image      *image.RGBA
	undo, redo [][]pixelChange
	pending    map[image.Point]pixelChange
	bytes      int
}

func NewPixelEditor(src *image.RGBA) *PixelEditor {
	return &PixelEditor{Image: cloneRGBA(src)}
}

func (e *PixelEditor) Begin() {
	if e.pending == nil {
		e.pending = make(map[image.Point]pixelChange)
	}
}

func (e *PixelEditor) Paint(p image.Point, radius int, c color.RGBA) {
	e.Begin()
	for y := p.Y - radius; y <= p.Y+radius; y++ {
		for x := p.X - radius; x <= p.X+radius; x++ {
			e.set(image.Pt(x, y), c)
		}
	}
}

func (e *PixelEditor) set(p image.Point, c color.RGBA) {
	if !p.In(e.Image.Bounds()) {
		return
	}
	before := e.Image.RGBAAt(p.X, p.Y)
	if before == c {
		return
	}
	change, ok := e.pending[p]
	if !ok {
		change = pixelChange{Point: p, Before: before}
	}
	change.After = c
	e.pending[p] = change
	e.Image.SetRGBA(p.X, p.Y, c)
}

func (e *PixelEditor) Line(from, to image.Point, radius int, c color.RGBA) {
	dx, dy := absInt(to.X-from.X), -absInt(to.Y-from.Y)
	sx, sy := 1, 1
	if from.X > to.X {
		sx = -1
	}
	if from.Y > to.Y {
		sy = -1
	}
	err := dx + dy
	for {
		e.Paint(from, radius, c)
		if from == to {
			break
		}
		twice := 2 * err
		if twice >= dy {
			err += dy
			from.X += sx
		}
		if twice <= dx {
			err += dx
			from.Y += sy
		}
	}
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func (e *PixelEditor) Fill(p image.Point, c color.RGBA) {
	if !p.In(e.Image.Bounds()) {
		return
	}
	old := e.Image.RGBAAt(p.X, p.Y)
	if old == c {
		return
	}
	e.Begin()
	e.set(p, c)
	queue := []image.Point{p}
	for len(queue) > 0 {
		point := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		for _, next := range []image.Point{point.Add(image.Pt(1, 0)), point.Add(image.Pt(-1, 0)), point.Add(image.Pt(0, 1)), point.Add(image.Pt(0, -1))} {
			if next.In(e.Image.Bounds()) && e.Image.RGBAAt(next.X, next.Y) == old {
				e.set(next, c)
				queue = append(queue, next)
			}
		}
	}
}

func (e *PixelEditor) Commit() {
	var changes []pixelChange
	for _, change := range e.pending {
		if change.Before != change.After {
			changes = append(changes, change)
		}
	}
	e.pending = nil
	if len(changes) == 0 {
		return
	}
	e.redo = nil
	e.undo = append(e.undo, changes)
	e.bytes += len(changes) * 24
	// Retain at least the latest action, even if it is a large fill.
	for len(e.undo) > 1 && (len(e.undo) > 100 || e.bytes > 64<<20) {
		e.bytes -= len(e.undo[0]) * 24
		e.undo = e.undo[1:]
	}
}

func (e *PixelEditor) Undo() {
	e.Commit()
	if len(e.undo) == 0 {
		return
	}
	changes := e.undo[len(e.undo)-1]
	e.undo = e.undo[:len(e.undo)-1]
	e.bytes -= len(changes) * 24
	for _, c := range changes {
		e.Image.SetRGBA(c.Point.X, c.Point.Y, c.Before)
	}
	e.redo = append(e.redo, changes)
}

func (e *PixelEditor) Redo() {
	if len(e.redo) == 0 {
		return
	}
	changes := e.redo[len(e.redo)-1]
	e.redo = e.redo[:len(e.redo)-1]
	for _, c := range changes {
		e.Image.SetRGBA(c.Point.X, c.Point.Y, c.After)
	}
	e.undo = append(e.undo, changes)
	e.bytes += len(changes) * 24
}

// Mapping includes the empty margins around a contained image. Clicking a
// margin never paints a border pixel. Crop dragging may clamp to an edge.
func containedImagePoint(bounds image.Rectangle, width, height, x, y float64, clamp bool) (image.Point, bool) {
	if bounds.Empty() || width <= 0 || height <= 0 {
		return image.Point{}, false
	}
	scale := min(width/float64(bounds.Dx()), height/float64(bounds.Dy()))
	left, top := (width-float64(bounds.Dx())*scale)/2, (height-float64(bounds.Dy())*scale)/2
	px, py := (x-left)/scale, (y-top)/scale
	inside := px >= 0 && py >= 0 && px < float64(bounds.Dx()) && py < float64(bounds.Dy())
	if !inside && !clamp {
		return image.Point{}, false
	}
	return image.Pt(min(bounds.Dx()-1, max(0, int(px))), min(bounds.Dy()-1, max(0, int(py)))), true
}
