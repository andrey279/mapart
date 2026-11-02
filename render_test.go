package main

import (
	"encoding/json"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"golang.org/x/image/draw"
)

func writeRenderFixture(t *testing.T, dir, name string, img image.Image) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := savePNG(path, img); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeRenderJSON(t *testing.T, dir, name, data string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRenderDetailMaskOverridesManualAndAutomaticSuppression(t *testing.T) {
	dir := t.TempDir()
	src := image.NewRGBA(image.Rect(0, 0, 128, 128))
	draw.Draw(src, src.Bounds(), image.NewUniform(color.RGBA{100, 150, 210, 255}), image.Point{}, draw.Src)
	draw.Draw(src, image.Rect(32, 32, 96, 96), image.NewUniform(color.RGBA{190, 110, 80, 255}), image.Point{}, draw.Src)
	noDither := image.NewGray(src.Bounds())
	draw.Draw(noDither, image.Rect(24, 24, 104, 104), image.NewUniform(color.White), image.Point{}, draw.Src)
	detail := image.NewGray(src.Bounds())
	draw.Draw(detail, image.Rect(48, 48, 80, 80), image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.Draw(detail, image.Rect(0, 0, 16, 16), image.NewUniform(color.White), image.Point{}, draw.Src)
	opts := DefaultRenderOptions()
	opts.MapsX, opts.MapsY, opts.Threshold = 1, 1, 0
	opts.NoDitherMask = writeRenderFixture(t, dir, "flat.png", noDither)
	opts.DetailMask = writeRenderFixture(t, dir, "detail.png", detail)
	got, err := RenderPreview(src, opts)
	if err != nil {
		t.Fatal(err)
	}
	for _, check := range []struct {
		x, y       int
		suppressed bool
		label      string
	}{
		{20, 20, true, "automatic plain background"},
		{40, 40, true, "manual mask on foreground"},
		{60, 60, false, "detail mask over manual mask"},
		{8, 8, false, "detail mask over automatic background"},
	} {
		if got.NoDitherMask[check.y*128+check.x] != check.suppressed {
			t.Errorf("incorrect priority for %s", check.label)
		}
	}
	colors := map[color.RGBA]bool{}
	for y := 52; y < 76; y++ {
		for x := 52; x < 76; x++ {
			colors[got.Image.RGBAAt(x, y)] = true
		}
	}
	if len(colors) < 2 {
		t.Fatal("detail mask changed metadata but did not restore foreground dithering")
	}
}

func TestRenderCustomPaletteAndMonochromeDoNotMutateDefaults(t *testing.T) {
	before := append([]MapColor(nil), palette...)
	dir := t.TempDir()
	opts := DefaultRenderOptions()
	opts.MapsX, opts.MapsY = 1, 1
	opts.Background, opts.SkyCleanup, opts.Dither = "off", false, "none"
	opts.PaletteFile = writeRenderJSON(t, dir, "palette.json", `["#112233","#FEDCBA"]`)
	src := image.NewRGBA(image.Rect(0, 0, 128, 128))
	draw.Draw(src, src.Bounds(), image.NewUniform(color.RGBA{17, 34, 51, 255}), image.Point{}, draw.Src)
	draw.Draw(src, image.Rect(64, 0, 128, 128), image.NewUniform(color.RGBA{254, 220, 186, 255}), image.Point{}, draw.Src)
	got, err := RenderPreview(src, opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Palette) != 2 || got.Image.RGBAAt(32, 64) != src.RGBAAt(32, 64) || got.Image.RGBAAt(96, 64) != src.RGBAAt(96, 64) {
		t.Fatal("custom palette file was not used by the renderer")
	}
	opts.PaletteFile, opts.Monochrome = "", true
	got, err = RenderPreview(src, opts)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range got.Palette {
		if p.R != p.G || p.G != p.B {
			t.Fatal("monochrome result reports a colored palette entry")
		}
	}
	for y := 0; y < 128; y++ {
		for x := 0; x < 128; x++ {
			p := got.Image.RGBAAt(x, y)
			if p.R != p.G || p.G != p.B {
				t.Fatal("monochrome was not applied to the actual image")
			}
		}
	}
	if !reflect.DeepEqual(palette, before) {
		t.Fatal("custom palette or monochrome permanently modified the global palette")
	}
}

func TestRenderDebugArtifactsDescribeActualProcessing(t *testing.T) {
	dir := t.TempDir()
	src := image.NewRGBA(image.Rect(0, 0, 32, 16))
	draw.Draw(src, src.Bounds(), image.NewUniform(color.RGBA{64, 64, 64, 255}), image.Point{}, draw.Src)
	draw.Draw(src, image.Rect(16, 0, 32, 16), image.NewUniform(color.RGBA{192, 192, 192, 255}), image.Point{}, draw.Src)
	manual := image.NewGray(src.Bounds())
	draw.Draw(manual, image.Rect(0, 0, 16, 16), image.NewUniform(color.White), image.Point{}, draw.Src)
	opts := DefaultRenderOptions()
	opts.MapsX, opts.MapsY, opts.Brightness = 1, 1, 10
	opts.Fit, opts.Resample, opts.Matte = "contain", "nearest", "#001122"
	opts.Background, opts.SkyCleanup = "off", false
	opts.NoDitherMask = writeRenderFixture(t, dir, "manual.png", manual)
	opts.PaletteFile = writeRenderJSON(t, dir, "custom.json", `["#000000","#ffffff"]`)
	base := filepath.Join(dir, "nested", "preview")
	if err := processConfiguredImage(src, base+".png", opts, true); err != nil {
		t.Fatal(err)
	}
	artifacts := map[string]image.Image{}
	for _, suffix := range []string{"", "_original", "_prepared", "_mask", "_mask_overlay"} {
		img, err := decodeLocalImage(base + suffix + ".png")
		if err != nil {
			t.Fatal(err)
		}
		if img.Bounds() != image.Rect(0, 0, 128, 128) {
			t.Fatalf("wrong dimensions in %s", suffix)
		}
		artifacts[suffix] = img
	}
	pixel := func(suffix string, x, y int) color.RGBA {
		return color.RGBAModel.Convert(artifacts[suffix].At(x, y)).(color.RGBA)
	}
	if pixel("_original", 10, 40).R != 64 || pixel("_prepared", 10, 40).R != 90 || pixel("_prepared", 110, 40).R != 218 {
		t.Fatal("original and prepared files do not show the requested brightness adjustment")
	}
	if pixel("_mask", 10, 40).R != 255 || pixel("_mask", 110, 40).R != 0 || pixel("_mask", 10, 0).R != 0 {
		t.Fatal("debug mask does not describe the actual source mask and contain padding")
	}
	if pixel("_mask_overlay", 10, 40) != (color.RGBA{45, 172, 45, 255}) || pixel("_mask_overlay", 110, 40) != pixel("_prepared", 110, 40) {
		t.Fatal("mask overlay does not reflect the selected pixels")
	}
	for y := 0; y < 128; y++ {
		for x := 0; x < 128; x++ {
			p := pixel("", x, y)
			if p != (color.RGBA{0, 0, 0, 255}) && p != (color.RGBA{255, 255, 255, 255}) {
				t.Fatal("saved preview contains a color outside its custom palette")
			}
		}
	}
	data, err := os.ReadFile(base + "_palette.json")
	if err != nil {
		t.Fatal(err)
	}
	var entries []string
	if err := json.Unmarshal(data, &entries); err != nil || !reflect.DeepEqual(entries, []string{"#000000", "#FFFFFF"}) {
		t.Fatalf("debug palette does not match the active custom palette: %s", data)
	}
	data, err = os.ReadFile(base + "_settings.json")
	if err != nil {
		t.Fatal(err)
	}
	var saved RenderOptions
	if err := json.Unmarshal(data, &saved); err != nil || saved != opts {
		t.Fatalf("debug settings do not describe the conversion: %s", data)
	}
}

func TestRenderRejectsInvalidMaskAndPaletteAssets(t *testing.T) {
	dir := t.TempDir()
	src := image.NewRGBA(image.Rect(0, 0, 8, 8))
	wrongSize := writeRenderFixture(t, dir, "wrong-size.png", image.NewGray(image.Rect(0, 0, 7, 8)))
	invalidImage := writeRenderJSON(t, dir, "not-image.png", "not an image")
	for _, path := range []string{filepath.Join(dir, "missing.png"), wrongSize, invalidImage} {
		for _, detail := range []bool{false, true} {
			opts := DefaultRenderOptions()
			opts.MapsX, opts.MapsY = 1, 1
			if detail {
				opts.DetailMask = path
			} else {
				opts.NoDitherMask = path
			}
			if _, err := RenderPreview(src, opts); err == nil {
				t.Errorf("accepted invalid mask %s, detail=%v", path, detail)
			}
		}
	}
	for _, fixture := range []struct {
		name, data string
		monochrome bool
	}{
		{"malformed", `{}`, false},
		{"one-color", `["#ffffff"]`, false},
		{"duplicates", `["#ffffff","#FFFFFF"]`, false},
		{"invalid-color", `["#000000","#zzzzzz"]`, false},
		{"no-neutral-colors", `["#112233","#abcdef"]`, true},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			opts := DefaultRenderOptions()
			opts.MapsX, opts.MapsY, opts.Monochrome = 1, 1, fixture.monochrome
			opts.PaletteFile = writeRenderJSON(t, dir, fixture.name+".json", fixture.data)
			if _, err := RenderPreview(src, opts); err == nil {
				t.Fatal("accepted invalid custom palette")
			}
		})
	}
	opts := DefaultRenderOptions()
	opts.PaletteFile = filepath.Join(dir, "missing-palette.json")
	if _, err := RenderPreview(src, opts); err == nil {
		t.Fatal("accepted missing palette file")
	}
}
