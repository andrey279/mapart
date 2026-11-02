package main

import (
	"errors"
	"flag"
	"image"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseCLIDefaultCompatibility(t *testing.T) {
	req, err := parseCLI([]string{"-i", "source.png"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if req.options != DefaultRenderOptions() {
		t.Fatalf("legacy defaults changed: %+v", req.options)
	}
	if req.input != "source.png" || req.preset != "balanced" || req.outDir != "output" || req.output != "" || req.debug || req.compare != "" {
		t.Fatalf("unexpected default request: %+v", req)
	}
	req, err = parseCLI([]string{"-i", "source.png", "-x", "2", "-y", "4", "-t", "0"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	expected := DefaultRenderOptions()
	expected.MapsX, expected.MapsY, expected.Threshold = 2, 4, 0
	if req.options != expected {
		t.Fatalf("legacy flags differ: got %+v", req.options)
	}
}

func TestParseCLIPresetConfigExplicitPrecedence(t *testing.T) {
	config := filepath.Join(t.TempDir(), "partial.json")
	data := `{"maps_x":2,"background":"auto","sky_cleanup":true,"clean_haze":true,"brightness":8,"saturation":1.1,"gamma":1.15,"dither_strength":0.55,"auto_contrast":true,"monochrome":true,"detail_mask":"details.png"}`
	if err := os.WriteFile(config, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	// The written order of flags must not change the layering order.
	req, err := parseCLI([]string{
		"-strength", "0", "-x", "4", "-background", "off",
		"-sky-cleanup=false", "-clean-haze=false", "-auto-contrast=false",
		"-monochrome=false", "-brightness", "0", "-saturation", "0",
		"-detail-mask", "", "-config", config, "-preset", "portrait",
	}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	expected, err := PresetOptions("portrait")
	if err != nil {
		t.Fatal(err)
	}
	expected.MapsX, expected.DitherStrength = 4, 0
	expected.Saturation, expected.Gamma = 0, 1.15
	if req.options != expected {
		t.Fatalf("wrong precedence or lost zero/false override:\n got %+v\nwant %+v", req.options, expected)
	}
	// Comparison changes the preset while retaining the same config and flags.
	pastel, err := req.resolve("pastel")
	if err != nil {
		t.Fatal(err)
	}
	expected.ColorMetric, expected.Threshold = "rgb", 0
	if pastel != expected {
		t.Fatalf("comparison lost layered overrides:\n got %+v\nwant %+v", pastel, expected)
	}
}

func TestParseCLIExplicitFalseOverridesPreset(t *testing.T) {
	req, err := parseCLI([]string{"-preset", "lineart", "-monochrome=false", "-auto-contrast=false", "-dither", "fs", "-strength", "0"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if req.options.Monochrome || req.options.AutoContrast || req.options.Dither != "fs" || req.options.DitherStrength != 0 {
		t.Fatalf("explicit values lost: %+v", req.options)
	}
}

func TestParseCLICompletesAndOverridesPartialConfig(t *testing.T) {
	for _, tc := range []struct {
		name string
		data string
		args []string
		want func(RenderOptions) bool
	}{
		{
			name: "color_completed_on_cli", data: `{"background":"color"}`,
			args: []string{"-bg-color", "#e0f0ff"},
			want: func(o RenderOptions) bool { return o.Background == "color" && o.BackgroundColor == "#e0f0ff" },
		},
		{
			name: "size_corrected_on_cli", data: `{"maps_x":32,"maps_y":16}`,
			args: []string{"-x", "1"},
			want: func(o RenderOptions) bool { return o.MapsX == 1 && o.MapsY == 16 },
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "partial.json")
			if err := os.WriteFile(path, []byte(tc.data), 0600); err != nil {
				t.Fatal(err)
			}
			// Public loading still rejects settings that are incomplete on their own.
			if _, err := LoadOptions(path, DefaultRenderOptions()); err == nil {
				t.Fatal("public LoadOptions skipped validation")
			}
			if _, err := parseCLI([]string{"-config", path}, io.Discard); err == nil {
				t.Fatal("CLI accepted an invalid final configuration")
			}
			args := append([]string{"-config", path}, tc.args...)
			req, err := parseCLI(args, io.Discard)
			if err != nil {
				t.Fatal(err)
			}
			if !tc.want(req.options) {
				t.Fatalf("CLI did not finish the configuration: %+v", req.options)
			}
		})
	}
}

func TestParseCLIErrorsAndHelp(t *testing.T) {
	for _, args := range [][]string{
		{"source.png"},
		{"-i", "source.png", "another.png"},
		{"-i", "source.png", "extra", "-x", "2"},
		{"-i", "source.png", "--", "extra"},
		{"-sky-cleanup", "false"},
		{"-preset", "unknown-preset"},
		{"-unknown-flag"},
		{"-i"},
		{"-strength", "NaN"},
		{"-gamma", "Inf"},
		{"-x", "0"},
		{"-dither", "unknown"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			if _, err := parseCLI(args, io.Discard); err == nil {
				t.Fatalf("accepted invalid arguments %v", args)
			}
		})
	}
	for _, help := range []string{"-h", "-help", "--help"} {
		if _, err := parseCLI([]string{help}, io.Discard); !errors.Is(err, flag.ErrHelp) {
			t.Errorf("%s returned %v instead of flag.ErrHelp", help, err)
		}
	}
}

func TestRunCLIModeValidation(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "tiny.png")
	f, err := os.Create(input)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"no_source", nil, "-i"},
		{"multiple_sources", []string{"-i", input, "-dir", dir}, "-i"},
		{"directory_single_output", []string{"-dir", dir, "-o", filepath.Join(dir, "out.png")}, "-o"},
		{"links_single_output", []string{"-links", filepath.Join(dir, "links.txt"), "-o", filepath.Join(dir, "out.png")}, "-o"},
		{"directory_compare", []string{"-dir", dir, "-compare", "all"}, "-compare"},
		{"links_compare", []string{"-links", filepath.Join(dir, "links.txt"), "-compare", "all"}, "-compare"},
		{"compare_single_output", []string{"-i", input, "-compare", "all", "-o", filepath.Join(dir, "out.png")}, "-compare"},
		{"non_png_output", []string{"-i", input, "-o", filepath.Join(dir, "out.jpg")}, "PNG"},
		{"config_does_not_hide_bad_output", []string{"-save-config", filepath.Join(dir, "invalid-output-config.json"), "-o", filepath.Join(dir, "out.png")}, "-o"},
		{"config_does_not_hide_bad_compare", []string{"-save-config", filepath.Join(dir, "invalid-compare-config.json"), "-compare", "all"}, "-compare"},
		{"invalid_modes_do_not_write", []string{"-i", input, "-dir", dir, "-save-config", filepath.Join(dir, "invalid-modes-config.json"), "-export-palette", filepath.Join(dir, "invalid-palette.json")}, "-i"},
		{"show_config_does_not_hide_bad_compare", []string{"-show-config", "-compare", "all"}, "-compare"},
		{"list_presets_does_not_hide_bad_output", []string{"-list-presets", "-o", filepath.Join(dir, "out.png")}, "-o"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := runCLI(tc.args)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("runCLI(%v) = %v; expected error mentioning %q", tc.args, err, tc.want)
			}
			for i, arg := range tc.args {
				if arg == "-save-config" || arg == "-export-palette" {
					if _, err := os.Stat(tc.args[i+1]); !os.IsNotExist(err) {
						t.Errorf("invalid command wrote %s before validating modes", tc.args[i+1])
					}
				}
			}
		})
	}
}

func TestRunCLICanSaveConfigWithoutImage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "profiles", "portrait.json")
	if err := runCLI([]string{"-preset", "portrait", "-save-config", path}); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(path); err != nil || info.Size() == 0 {
		t.Fatalf("configuration was not written: %v", err)
	}
}
