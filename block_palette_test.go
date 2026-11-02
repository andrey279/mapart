package main

import (
	"image"
	"image/color"
	"io"
	"math"
	"reflect"
	"testing"
)

func TestTerracottaStructureColorsAndBlocks(t *testing.T) {
	// Independently specified Java base RGBs, in dye order.
	wantBase := []int{0xd1b1a1, 0x9f5224, 0x95576c, 0x706c8a, 0xba8524, 0x677535, 0xa04d4e, 0x392923,
		0x876b62, 0x575c5c, 0x7a4958, 0x4c3e5c, 0x4c3223, 0x4c522a, 0x8e3c2e, 0x251610}
	names := []string{"white", "orange", "magenta", "light_blue", "yellow", "lime", "pink", "gray", "light_gray", "cyan", "purple", "blue", "brown", "green", "red", "black"}
	opts := DefaultRenderOptions()
	opts.Terracotta = true
	colors, err := paletteForOptions(opts)
	if err != nil || len(colors) != 96 {
		t.Fatalf("palette: %d, %v", len(colors), err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 16, 3))
	for x := 0; x < 16; x++ {
		for y := 0; y < 3; y++ {
			p := colors[48+x*3+y]
			img.SetRGBA(x, y, color.RGBA{uint8(math.Round(p.R)), uint8(math.Round(p.G)), uint8(math.Round(p.B)), 255})
		}
	}
	s, err := BuildStructure(&PreviewResult{Image: img, Palette: colors}, DefaultBuildOptions())
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range s.Blocks {
		if b.Z == 0 || b.Y != s.Heights[b.Z*16+b.X] {
			continue
		}
		if s.Palette[b.State] != "minecraft:"+names[b.X]+"_terracotta" {
			t.Fatalf("wrong block at %v: %s", b, s.Palette[b.State])
		}
		delta := b.Y - s.Heights[(b.Z-1)*16+b.X]
		value := float64(delta)*4/5 + (float64((b.X+b.Z)&1)-.5)*.4
		modifier := 220
		if value > .6 {
			modifier = 255
		} else if value < -.6 {
			modifier = 180
		}
		base := wantBase[b.X]
		want := color.RGBA{uint8((base >> 16 & 255) * modifier / 255), uint8((base >> 8 & 255) * modifier / 255), uint8((base & 255) * modifier / 255), 255}
		if got := s.MapImage.RGBAAt(b.X, b.Z-1); got != want || img.RGBAAt(b.X, b.Z-1) != want {
			t.Fatalf("shade mismatch at %v: got %v, want %v", b, got, want)
		}
	}
	legacy, err := paletteForOptions(DefaultRenderOptions())
	if err != nil || !reflect.DeepEqual(legacy, palette) || !reflect.DeepEqual(colors[:48], palette) {
		t.Fatal("expanded palette changed legacy wool")
	}
	opts.PaletteFile = "custom.json"
	if ValidateOptions(opts) == nil {
		t.Fatal("accepted conflicting palettes")
	}
}

func TestTerracottaCLIAndProfile(t *testing.T) {
	req, err := parseCLI([]string{"-terracotta"}, io.Discard)
	if err != nil || !req.options.Terracotta {
		t.Fatalf("CLI: %+v, %v", req.options, err)
	}
	path := t.TempDir() + "/profile.json"
	if err := SaveOptions(path, req.options); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadOptions(path, DefaultRenderOptions())
	if err != nil || loaded != req.options {
		t.Fatalf("profile roundtrip: %v", err)
	}
}
