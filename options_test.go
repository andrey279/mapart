package main

import (
	"encoding/json"
	"image"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestRenderOptionsDefaultsAndPresets(t *testing.T) {
	defaults := DefaultRenderOptions()
	if err := ValidateOptions(defaults); err != nil {
		t.Fatal(err)
	}
	if defaults.MapsX != 5 || defaults.MapsY != 3 || defaults.Threshold != 5 || defaults.Dither != "fs" || defaults.DitherStrength != 0.65 || defaults.ColorMetric != "lab" || defaults.Background != "auto" || defaults.BackgroundTolerance != 45 || !defaults.SkyCleanup || defaults.CleanHaze {
		t.Fatalf("existing rendering defaults changed: %+v", defaults)
	}
	if defaults.Brightness != 0 || defaults.Contrast != 1 || defaults.Saturation != 1 || defaults.Gamma != 1 || defaults.Temperature != 0 || defaults.Sharpen != 0 || defaults.Denoise != 0 || defaults.AutoContrast || defaults.Monochrome || defaults.Fit != "stretch" || defaults.Resample != "bicubic" || defaults.Matte != "#ffffff" {
		t.Fatalf("default preprocessing is not neutral: %+v", defaults)
	}
	for _, name := range PresetNames() {
		t.Run(name, func(t *testing.T) {
			opts, err := PresetOptions(name)
			if err != nil {
				t.Fatal(err)
			}
			if err := opts.Validate(); err != nil {
				t.Fatal(err)
			}
			if PresetDescription(name) == "" {
				t.Fatal("missing user-facing description")
			}
			if name == "balanced" && opts != defaults {
				t.Fatal("balanced differs from defaults")
			}
			if (name == "portrait" || name == "pastel") && (opts.Background != "off" || opts.SkyCleanup || opts.CleanHaze) {
				t.Fatal("pastel detail unexpectedly suppressed")
			}
		})
	}
	if _, err := PresetOptions("does-not-exist"); err == nil {
		t.Fatal("accepted unknown preset")
	}
}

func TestRenderOptionsValidation(t *testing.T) {
	cases := []struct {
		name   string
		change func(*RenderOptions)
	}{
		{"maps_zero", func(o *RenderOptions) { o.MapsX = 0 }},
		{"maps_negative", func(o *RenderOptions) { o.MapsY = -1 }},
		{"maps_total", func(o *RenderOptions) { o.MapsX, o.MapsY = 17, 16 }},
		{"maps_overflow", func(o *RenderOptions) { o.MapsX, o.MapsY = int(^uint(0)>>1), 2 }},
		{"threshold_negative", func(o *RenderOptions) { o.Threshold = -1 }},
		{"threshold_nan", func(o *RenderOptions) { o.Threshold = math.NaN() }},
		{"strength_large", func(o *RenderOptions) { o.DitherStrength = 1.1 }},
		{"strength_negative", func(o *RenderOptions) { o.DitherStrength = -0.1 }},
		{"tolerance_large", func(o *RenderOptions) { o.BackgroundTolerance = 151 }},
		{"brightness_large", func(o *RenderOptions) { o.Brightness = 101 }},
		{"brightness_small", func(o *RenderOptions) { o.Brightness = -101 }},
		{"contrast_zero", func(o *RenderOptions) { o.Contrast = 0 }},
		{"contrast_large", func(o *RenderOptions) { o.Contrast = 3.1 }},
		{"saturation_small", func(o *RenderOptions) { o.Saturation = -0.1 }},
		{"saturation_large", func(o *RenderOptions) { o.Saturation = 3.1 }},
		{"gamma_small", func(o *RenderOptions) { o.Gamma = 0.1 }},
		{"gamma_large", func(o *RenderOptions) { o.Gamma = 3.1 }},
		{"temperature_small", func(o *RenderOptions) { o.Temperature = -101 }},
		{"temperature_large", func(o *RenderOptions) { o.Temperature = 101 }},
		{"sharpen_small", func(o *RenderOptions) { o.Sharpen = -0.1 }},
		{"sharpen_large", func(o *RenderOptions) { o.Sharpen = 2.1 }},
		{"denoise_small", func(o *RenderOptions) { o.Denoise = -0.1 }},
		{"denoise_large", func(o *RenderOptions) { o.Denoise = 1.1 }},
		{"bad_dither", func(o *RenderOptions) { o.Dither = "random" }},
		{"bad_metric", func(o *RenderOptions) { o.ColorMetric = "cie" }},
		{"bad_bg", func(o *RenderOptions) { o.Background = "on" }},
		{"bad_fit", func(o *RenderOptions) { o.Fit = "cropped" }},
		{"bad_resample", func(o *RenderOptions) { o.Resample = "smooth" }},
		{"bad_matte", func(o *RenderOptions) { o.Matte = "#zzffff" }},
		{"color_without_color", func(o *RenderOptions) { o.Background = "color" }},
		{"bad_background_color", func(o *RenderOptions) { o.BackgroundColor = "cyan" }},
		{"bad_crop", func(o *RenderOptions) { o.Crop = "0,0,0,2" }},
		{"bad_path", func(o *RenderOptions) { o.DetailMask = "a\x00b" }},
	}
	for _, field := range []string{"Threshold", "DitherStrength", "BackgroundTolerance", "Brightness", "Contrast", "Saturation", "Gamma", "Temperature", "Sharpen", "Denoise"} {
		for _, nonfinite := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
			o := DefaultRenderOptions()
			reflect.ValueOf(&o).Elem().FieldByName(field).SetFloat(nonfinite)
			if err := o.Validate(); err == nil {
				t.Errorf("accepted %s=%v", field, nonfinite)
			}
		}
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts := DefaultRenderOptions()
			tc.change(&opts)
			if err := ValidateOptions(opts); err == nil {
				t.Fatalf("accepted invalid options: %+v", opts)
			}
		})
	}
	o := DefaultRenderOptions()
	o.MapsX, o.MapsY = 256, 1
	o.Threshold, o.DitherStrength, o.Saturation, o.BackgroundTolerance = 0, 0, 0, 0
	o.Background, o.BackgroundColor = "color", "#00fF8A"
	if err := o.Validate(); err != nil {
		t.Fatalf("rejected valid boundaries: %v", err)
	}
}

func TestRenderOptionsLoadMergesZerosFalseAndRelativePaths(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "preset.json")
	data := `{"dither_strength":0,"threshold":0,"saturation":0,"sky_cleanup":false,"clean_haze":false,"auto_contrast":false,"monochrome":false,"palette_file":"assets/palette.json","no_dither_mask":"masks/flat.png","detail_mask":"detail.png"}`
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	base := DefaultRenderOptions()
	base.CleanHaze, base.AutoContrast, base.Monochrome = true, true, true
	base.MapsX = 4
	opts, err := LoadOptions(path, base)
	if err != nil {
		t.Fatal(err)
	}
	if opts.DitherStrength != 0 || opts.Threshold != 0 || opts.Saturation != 0 || opts.SkyCleanup || opts.CleanHaze || opts.AutoContrast || opts.Monochrome {
		t.Fatalf("explicit zero/false overridden: %+v", opts)
	}
	if opts.MapsX != 4 || opts.MapsY != base.MapsY || opts.Contrast != base.Contrast || opts.Fit != base.Fit {
		t.Fatal("unspecified base fields lost")
	}
	if opts.PaletteFile != filepath.Join(dir, "assets", "palette.json") || opts.NoDitherMask != filepath.Join(dir, "masks", "flat.png") || opts.DetailMask != filepath.Join(dir, "detail.png") {
		t.Fatalf("paths not relative to config: %+v", opts)
	}
	if err := os.WriteFile(path, []byte(`{"detail_mask":""}`), 0600); err != nil {
		t.Fatal(err)
	}
	base.PaletteFile, base.DetailMask = "inherited-relative.json", "clear-me.png"
	opts, err = LoadOptions(path, base)
	if err != nil {
		t.Fatal(err)
	}
	if opts.PaletteFile != base.PaletteFile || opts.DetailMask != "" {
		t.Fatal("load reinterpreted an inherited path or lost explicit empty string")
	}
}

func TestRenderOptionsLoadStrictJSON(t *testing.T) {
	for _, data := range []string{
		`{"typo":true}`, `{"MapsX":3}`, `{"maps_x":2,"maps_x":3}`,
		`{"maps_x":null}`, `{"sky_cleanup":null}`, `null`, `[]`, `"balanced"`,
		`{} {}`, `{} null`, `{} garbage`, `{"maps_x":2`, `{"maps_x":2,}`,
		`{"threshold":NaN}`, `{"threshold":Infinity}`, `{"threshold":1e999}`,
		`{"maps_x":1.5}`, `{"sky_cleanup":"false"}`, `{"dither":"error-diffusion"}`,
		`{"maps_x":0}`, `{"dither_strength":-1}`, `{"gamma":0}`,
	} {
		t.Run(data, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "bad.json")
			if err := os.WriteFile(path, []byte(data), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadOptions(path, DefaultRenderOptions()); err == nil {
				t.Fatalf("accepted malformed configuration %s", data)
			}
		})
	}
}

func TestRenderOptionsSaveResolvedRoundTrip(t *testing.T) {
	opts := DefaultRenderOptions()
	opts.SkyCleanup, opts.DitherStrength = false, 0
	opts.PaletteFile = filepath.Join(t.TempDir(), "palette.json")
	opts.DetailMask = "relative-detail.png"
	path := filepath.Join(t.TempDir(), "nested", "profiles", "saved.json")
	if err := SaveOptions(path, opts); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	if len(fields) != reflect.TypeOf(RenderOptions{}).NumField() || string(fields["sky_cleanup"]) != "false" || string(fields["dither_strength"]) != "0" {
		t.Fatalf("saved configuration is incomplete: %s", data)
	}
	loaded, err := LoadOptions(path, RenderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	expected := opts
	expected.DetailMask, err = filepath.Abs(expected.DetailMask)
	if err != nil {
		t.Fatal(err)
	}
	if loaded != expected {
		t.Fatalf("round-trip differs: got %+v; expected %+v", loaded, expected)
	}
	invalid := opts
	invalid.Contrast = 0
	if err := SaveOptions(path, invalid); err == nil {
		t.Fatal("saved invalid configuration")
	}
	unchanged, err := os.ReadFile(path)
	if err != nil || string(unchanged) != string(data) {
		t.Fatal("validation failure altered existing file")
	}
}

func TestRenderOptionColorAndCropParsing(t *testing.T) {
	c, err := ParseHexColor("#00fF8A")
	if err != nil || c != (color.RGBA{0, 255, 138, 255}) {
		t.Fatalf("ParseHexColor = %v, %v", c, err)
	}
	for _, value := range []string{"", "ffffff", "#fff", "#gg0000", "#ffffffff", " #ffffff"} {
		if _, err := ParseHexColor(value); err == nil {
			t.Errorf("accepted %q", value)
		}
	}
	r, err := ParseCrop("10, 20, 30, 40")
	if err != nil || r != image.Rect(10, 20, 40, 60) {
		t.Fatalf("ParseCrop = %v, %v", r, err)
	}
	r, err = ParseCrop("")
	if err != nil || r != (image.Rectangle{}) {
		t.Fatal("empty crop not accepted")
	}
	for _, value := range []string{"1,2,3", "-1,0,3,4", "0,0,0,4", "0,0,3,-1", "a,0,1,1", "0,0,1,1,2", strings.Repeat("9", 40) + ",0,1,1", "9223372036854775807,0,1,1"} {
		if _, err := ParseCrop(value); err == nil {
			t.Errorf("accepted crop %q", value)
		}
	}
}
