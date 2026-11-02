package main

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultDitheringPreservesPaleSkinWarmth(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 128, 128))
	for y := 0; y < 128; y++ {
		for x := 0; x < 128; x++ {
			p := color.RGBA{50, 100, 180, 255}
			if (x/8+y/8)%2 == 1 {
				p = color.RGBA{180, 100, 50, 255}
			}
			if x >= 16 && x < 112 && y >= 16 && y < 112 {
				p = color.RGBA{250, 224, 202, 255}
			}
			src.SetRGBA(x, y, p)
		}
	}
	region := image.Rect(24, 24, 104, 104)
	mask, _ := detectBackground(src)
	for y := region.Min.Y; y < region.Max.Y; y++ {
		for x := region.Min.X; x < region.Max.X; x++ {
			if mask[y*128+x] {
				t.Fatal("skin fixture was incorrectly classified as background")
			}
		}
	}

	counts := [2]int{}
	for mode := 0; mode < 2; mode++ {
		out := filepath.Join(t.TempDir(), "preview.png")
		var err error
		if mode == 0 {
			// Exercise the public default, so accidentally enabling cleanup here
			// cannot silently turn pale skin into a gray patch again.
			err = processImage(src, out, 1, 1, 5)
		} else {
			err = processImageWithCleanup(src, out, 1, 1, 5, true)
		}
		if err != nil {
			t.Fatal(err)
		}
		f, err := os.Open(out)
		if err != nil {
			t.Fatal(err)
		}
		got, err := png.Decode(f)
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
		for y := region.Min.Y; y < region.Max.Y; y++ {
			for x := region.Min.X; x < region.Max.X; x++ {
				p := color.RGBAModel.Convert(got.At(x, y)).(color.RGBA)
				if int(p.R)-int(p.B) > 40 {
					counts[mode]++
				}
			}
		}
	}
	minimum := region.Dx() * region.Dy() / 50
	t.Logf("default %d warm pixels, optional cleanup %d", counts[0], counts[1])
	if counts[0] < minimum {
		t.Errorf("default lost pale skin warmth: %d warm pixels, need at least %d", counts[0], minimum)
	}
}
