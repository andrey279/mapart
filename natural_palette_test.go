package main

import (
	"image"
	"image/color"
	"io"
	"math"
	"reflect"
	"testing"
)

func TestNaturalBlockShadesAndStructure(t *testing.T) {
	// Independently read from Mojang's Java 1.20.1 MapColor and Blocks classes.
	names := []string{"clay", "prismarine_bricks", "packed_ice", "sandstone", "stone", "oak_planks"}
	base := []int{0xa4a8b8, 0x5cdbd5, 0xa0a0ff, 0xf7e9a3, 0x707070, 0x8f7748}
	opts := DefaultRenderOptions()
	opts.Terracotta, opts.NaturalBlocks = true, true
	colors, err := paletteForOptions(opts)
	if err != nil || len(colors) != 114 {
		t.Fatal("expanded palette", len(colors), err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 6, 3))
	for x := 0; x < 6; x++ {
		for y := 0; y < 3; y++ {
			img.SetRGBA(x, y, palettePixel(colors[96+x*3+y]))
		}
	}
	s, err := BuildStructure(&PreviewResult{Image: img, Palette: colors}, DefaultBuildOptions())
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range s.Blocks {
		if b.Z == 0 || b.Y != s.Heights[b.Z*6+b.X] {
			continue
		}
		if s.Palette[b.State] != "minecraft:"+names[b.X] {
			t.Fatal("wrong natural block", b, s.Palette[b.State])
		}
		delta := float64(b.Y-s.Heights[(b.Z-1)*6+b.X])*4/5 + (float64((b.X+b.Z-1)&1)-.5)*.4
		mod := 220
		if delta > .6 {
			mod = 255
		} else if delta < -.6 {
			mod = 180
		}
		value := base[b.X]
		want := color.RGBA{uint8((value >> 16 & 255) * mod / 255), uint8((value >> 8 & 255) * mod / 255), uint8((value & 255) * mod / 255), 255}
		if s.MapImage.RGBAAt(b.X, b.Z-1) != want || img.RGBAAt(b.X, b.Z-1) != want {
			t.Fatal("natural block shade differs from Minecraft", b)
		}
	}
	legacy, err := paletteForOptions(DefaultRenderOptions())
	if err != nil || !reflect.DeepEqual(legacy, palette) || !reflect.DeepEqual(colors[:48], palette) {
		t.Fatal("legacy wool palette changed", err)
	}
	opts.SelectedBlocks = "clay,sandstone,stone"
	opts.BuildMode = "2d"
	flat, err := paletteForOptions(opts)
	if err != nil || len(flat) != 3 {
		t.Fatal("flat natural selection", flat, err)
	}
	for _, p := range flat {
		if p.Shade != "Normal" {
			t.Fatal("flat palette has a height shade")
		}
	}
	opts.PaletteFile = "custom.json"
	if err := ValidateOptions(opts); err == nil {
		t.Fatal("accepted incompatible natural/RGB palettes")
	}
}

func TestNaturalPalettePreservesPaleBlueAverage(t *testing.T) {
	const width, height = 128, 128
	target := color.RGBA{143, 185, 225, 255}
	source := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			source.SetRGBA(x, y, target)
		}
	}
	opts, _ := PresetOptions("landscape")
	opts.Terracotta = true
	average := func(natural bool) [3]float64 {
		opts.NaturalBlocks = natural
		colors, err := paletteForOptions(opts)
		if err != nil {
			t.Fatal(err)
		}
		out, err := QuantizeImage(source, nil, nil, opts, colors)
		if err != nil {
			t.Fatal(err)
		}
		var result [3]float64
		// Exclude the diffusion start and outer edge from this colour measurement.
		for y := 16; y < 112; y++ {
			for x := 16; x < 112; x++ {
				c := out.RGBAAt(x, y)
				result[0] += float64(c.R)
				result[1] += float64(c.G)
				result[2] += float64(c.B)
			}
		}
		for ch := range result {
			result[ch] /= 96 * 96
		}
		return result
	}
	old, improved := average(false), average(true)
	for ch, want := range []float64{143, 185, 225} {
		if math.Abs(improved[ch]-want) > 2 {
			t.Fatalf("pale sky average shifted: %v", improved)
		}
	}
	if math.Abs(improved[1]-185) >= math.Abs(old[1]-185)/2 {
		t.Fatalf("natural colours did not improve sky colour: old=%v new=%v", old, improved)
	}
}

func TestNaturalPaletteCLIProfilesAndPresets(t *testing.T) {
	req, err := parseCLI([]string{"-preset", "landscape", "-natural-blocks", "-terracotta"}, io.Discard)
	if err != nil || !req.options.NaturalBlocks {
		t.Fatal("natural CLI", req, err)
	}
	path := t.TempDir() + "/profile.json"
	if err := SaveOptions(path, req.options); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadOptions(path, DefaultRenderOptions())
	if err != nil || loaded != req.options {
		t.Fatal("natural profile round trip", err)
	}
	preset, _ := PresetOptions("portrait")
	if preserved := preservePresetAssets(preset, loaded); !preserved.NaturalBlocks || !preserved.Terracotta {
		t.Fatal("preset lost selected palette")
	}
}
