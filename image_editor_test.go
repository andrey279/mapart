package main

import (
	"image"
	"image/color"
	"image/draw"
	"testing"
)

func TestSourceRotationMirrorAndTransparency(t *testing.T) {
	src := image.NewNRGBA(image.Rect(10, 20, 12, 23))
	for y := 0; y < 3; y++ {
		for x := 0; x < 2; x++ {
			src.SetNRGBA(10+x, 20+y, color.NRGBA{uint8(y*2 + x + 1), 40, 80, 128})
		}
	}
	cases := []struct {
		rotation             int
		horizontal, vertical bool
		rows                 [][]uint8
	}{
		{90, false, false, [][]uint8{{5, 3, 1}, {6, 4, 2}}},
		{180, false, false, [][]uint8{{6, 5}, {4, 3}, {2, 1}}},
		{270, false, false, [][]uint8{{2, 4, 6}, {1, 3, 5}}},
		{90, true, false, [][]uint8{{1, 3, 5}, {2, 4, 6}}},
		{90, false, true, [][]uint8{{6, 4, 2}, {5, 3, 1}}},
	}
	for _, c := range cases {
		opts := DefaultRenderOptions()
		opts.Rotation = c.rotation
		opts.FlipHorizontal = c.horizontal
		opts.FlipVertical = c.vertical
		got := transformSource(src, opts)
		for y, row := range c.rows {
			for x, v := range row {
				p := color.NRGBAModel.Convert(got.At(x, y)).(color.NRGBA)
				if p.R != v || p.A != 128 {
					t.Fatalf("rotation %d mirror %v,%v pixel %d,%d: %+v", c.rotation, c.horizontal, c.vertical, x, y, p)
				}
			}
		}
	}
}

func TestRotatedCroppedSourceMaskMatchesImage(t *testing.T) {
	src := image.NewRGBA(image.Rect(10, 20, 16, 24))
	mask := image.NewGray(image.Rect(0, 0, 6, 4))
	draw.Draw(src, src.Bounds(), image.NewUniform(color.Black), image.Point{}, draw.Src)
	for y := 0; y < 3; y++ {
		for x := 1; x < 4; x++ {
			src.SetRGBA(10+x, 20+y, color.RGBA{255, 255, 255, 255})
			mask.SetGray(x, y, color.Gray{255})
		}
	}
	for _, fit := range []string{"stretch", "contain", "cover"} {
		opts := DefaultRenderOptions()
		opts.MapsX, opts.MapsY = 1, 1
		opts.Rotation = 90
		opts.FlipHorizontal = true
		opts.FlipVertical = true
		opts.Crop = "0,0,3,4"
		opts.Fit = fit
		opts.Resample = "nearest"
		opts.Matte = "#000000"
		prepared, err := PrepareImage(src, opts)
		if err != nil {
			t.Fatal(err)
		}
		selected, err := TransformMask(mask, src.Bounds(), opts)
		if err != nil {
			t.Fatal(err)
		}
		for y := 0; y < 128; y++ {
			for x := 0; x < 128; x++ {
				if (prepared.RGBAAt(x, y).R == 255) != selected[y*128+x] {
					t.Fatalf("mask detached from transformed source with %s at %d,%d", fit, x, y)
				}
			}
		}
	}
}

func TestPixelEditorStrokesUndoRedoAndBranch(t *testing.T) {
	src := image.NewRGBA(image.Rect(5, 8, 25, 28))
	draw.Draw(src, src.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	editor := NewPixelEditor(src)
	black := color.RGBA{0, 0, 0, 255}
	red := color.RGBA{255, 0, 0, 255}
	editor.Line(image.Pt(1, 1), image.Pt(15, 15), 0, black)
	editor.Commit()
	for i := 1; i <= 15; i++ {
		if editor.Image.RGBAAt(i, i) != black {
			t.Fatal("stroke skipped a diagonal pixel")
		}
	}
	if src.RGBAAt(6, 9) != (color.RGBA{255, 255, 255, 255}) {
		t.Fatal("editor changed its source")
	}
	editor.Paint(image.Pt(5, 5), 1, red)
	editor.Paint(image.Pt(5, 5), 1, black)
	editor.Commit()
	editor.Undo()
	if editor.Image.RGBAAt(4, 5) != (color.RGBA{255, 255, 255, 255}) || editor.Image.RGBAAt(5, 5) != black {
		t.Fatal("undo did not restore pixels before the whole stroke")
	}
	editor.Redo()
	if editor.Image.RGBAAt(4, 5) != black {
		t.Fatal("redo lost a brush stroke")
	}
	editor.Undo()
	editor.Undo()
	if editor.Image.RGBAAt(1, 1) != (color.RGBA{255, 255, 255, 255}) {
		t.Fatal("first stroke was not undone")
	}
	editor.Paint(image.Pt(0, 0), 0, red)
	editor.Commit()
	editor.Redo()
	if editor.Image.RGBAAt(1, 1) != (color.RGBA{255, 255, 255, 255}) {
		t.Fatal("new edit retained the old redo branch")
	}
}

func TestPixelEditorFillRespectsBoundariesAndNoOp(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 8, 8))
	draw.Draw(src, src.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	black := color.RGBA{0, 0, 0, 255}
	red := color.RGBA{255, 0, 0, 255}
	for y := 0; y < 8; y++ {
		src.SetRGBA(4, y, black)
	}
	e := NewPixelEditor(src)
	e.Fill(image.Pt(0, 0), red)
	e.Commit()
	if e.Image.RGBAAt(3, 7) != red || e.Image.RGBAAt(4, 7) != black || e.Image.RGBAAt(7, 7) != (color.RGBA{255, 255, 255, 255}) {
		t.Fatal("fill crossed a boundary")
	}
	e.Fill(image.Pt(0, 0), red)
	e.Paint(image.Pt(-20, -20), 0, red)
	e.Commit()
	e.Undo()
	if e.Image.RGBAAt(0, 0) != (color.RGBA{255, 255, 255, 255}) {
		t.Fatal("no-op generated an undo action")
	}
	e.Redo()
	if e.Image.RGBAAt(0, 0) != red {
		t.Fatal("fill cannot be redone")
	}
}

func TestContainedImageCoordinates(t *testing.T) {
	b := image.Rect(0, 0, 200, 100)
	if _, ok := containedImagePoint(b, 400, 400, 10, 10, false); ok {
		t.Fatal("margin paints a pixel")
	}
	p, ok := containedImagePoint(b, 400, 400, 399, 299, false)
	if !ok || p != image.Pt(199, 99) {
		t.Fatalf("wrong image coordinate %v %v", p, ok)
	}
	p, ok = containedImagePoint(b, 400, 400, 500, 500, true)
	if !ok || p != image.Pt(199, 99) {
		t.Fatal("crop did not clamp to the edge")
	}
}
