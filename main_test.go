package main

import (
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestRGBToLabGray(t *testing.T) {
	l, a, b := rgbToLab(128, 128, 128)
	if math.Abs(l-53.585) > 0.01 || math.Abs(a) > 0.02 || math.Abs(b) > 0.02 {
		t.Fatalf("Lab(128,128,128) = (%f,%f,%f)", l, a, b)
	}
}

func TestTransparentImageDoesNotGainStripes(t *testing.T) {
	// A transparent source becomes a uniform white map, including below the
	// row where the former height-reset code produced diagonal dark stripes.
	src := image.NewNRGBA(image.Rect(10, 20, 11, 21))
	src.SetNRGBA(10, 20, color.NRGBA{R: 255, A: 0})
	out := filepath.Join(t.TempDir(), "preview.png")
	if err := processImage(src, out, 1, 1, 5); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	got, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	if got.Bounds().Dx() != 128 || got.Bounds().Dy() != 128 {
		t.Fatalf("unexpected dimensions: %v", got.Bounds())
	}
	for y := 0; y < 128; y++ {
		for x := 0; x < 128; x++ {
			r, g, b, _ := got.At(x, y).RGBA()
			if r != 0xffff || g != 0xffff || b != 0xffff {
				t.Fatalf("unexpected pixel at (%d,%d): %04x,%04x,%04x", x, y, r, g, b)
			}
		}
	}
}

func TestInvalidMapDimensions(t *testing.T) {
	for _, size := range [][2]int{{0, 1}, {1, -1}, {257, 1}, {17, 16}} {
		if err := processImage(image.NewRGBA(image.Rect(0, 0, 1, 1)), "unused.png", size[0], size[1], 5); err == nil {
			t.Fatalf("accepted dimensions %v", size)
		}
	}
}

func TestBackgroundIsFlatAndForegroundStillDithers(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 128, 128))
	for y := 0; y < 128; y++ {
		for x := 0; x < 128; x++ {
			p := color.RGBA{100, 150, 210, 255}
			if x >= 32 && x < 96 && y >= 32 && y < 96 {
				p = color.RGBA{190, 110, 80, 255}
			}
			src.SetRGBA(x, y, p)
		}
	}
	mask, _ := detectBackground(src)
	if !mask[10*128+10] || mask[64*128+64] {
		t.Fatal("background mask included the foreground or missed the background")
	}
	out := filepath.Join(t.TempDir(), "preview.png")
	if err := processImage(src, out, 1, 1, 0); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	got, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	backgroundColor := got.At(0, 0)
	for y := 0; y < 128; y++ {
		for x := 0; x < 128; x++ {
			if x < 25 || x >= 103 || y < 25 || y >= 103 {
				if got.At(x, y) != backgroundColor {
					t.Fatalf("background has dithering at (%d,%d)", x, y)
				}
			}
		}
	}
	foregroundColors := map[color.Color]bool{}
	for y := 40; y < 88; y++ {
		for x := 40; x < 88; x++ {
			foregroundColors[got.At(x, y)] = true
		}
	}
	if len(foregroundColors) < 2 {
		t.Fatal("foreground was not dithered")
	}
}

func TestEnclosedBackgroundGap(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 128, 128))
	for y := 0; y < 128; y++ {
		for x := 0; x < 128; x++ {
			p := color.RGBA{80, 140, 200, 255}
			if x >= 32 && x < 96 && y >= 32 && y < 96 && !(x >= 48 && x < 80 && y >= 48 && y < 80) {
				p = color.RGBA{220, 110, 60, 255}
			}
			src.SetRGBA(x, y, p)
		}
	}
	mask, _ := detectBackground(src)
	if !mask[64*128+64] || mask[40*128+40] {
		t.Fatal("enclosed sky gap was not selected correctly")
	}
}

func TestDetailedBorderKeepsDithering(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 128, 128))
	for y := 0; y < 128; y++ {
		for x := 0; x < 128; x++ {
			p := color.RGBA{50, 100, 180, 255}
			if (x/16+y/16)%2 == 1 {
				p = color.RGBA{180, 100, 50, 255}
			}
			src.SetRGBA(x, y, p)
		}
	}
	mask, _ := detectBackground(src)
	for _, selected := range mask {
		if selected {
			t.Fatal("textured border was incorrectly treated as a plain background")
		}
	}
}
