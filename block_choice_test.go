package main

import (
	"archive/zip"
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"io"
	"path/filepath"
	"strings"
	"testing"
)

func TestBlockSelectionControlsPaletteAndMaterials(t *testing.T) {
	opts := DefaultRenderOptions()
	opts.MapsX, opts.MapsY, opts.Dither = 1, 1, "none"
	opts.SelectedBlocks = "white_wool,black_terracotta"
	for _, mode := range []string{"2d", "3d"} {
		t.Run(mode, func(t *testing.T) {
			opts.BuildMode = mode
			src := image.NewRGBA(image.Rect(0, 0, 16, 16))
			for y := 0; y < 16; y++ {
				for x := 0; x < 16; x++ {
					src.SetRGBA(x, y, color.RGBA{uint8(x * 16), uint8(y * 16), 80, 255})
				}
			}
			result, err := RenderPreview(src, opts)
			if err != nil {
				t.Fatal(err)
			}
			want := 6
			if mode == "2d" {
				want = 2
			}
			if len(result.Palette) != want {
				t.Fatalf("palette has %d colours, want %d", len(result.Palette), want)
			}
			for _, p := range result.Palette {
				if p.WoolID != 0 && p.WoolID != 31 {
					t.Fatal("palette contains an unselected block")
				}
				if mode == "2d" && p.Shade != "Normal" {
					t.Fatal("flat preview has an unattainable shade")
				}
			}
			s, err := BuildStructure(result, DefaultBuildOptions())
			if err != nil {
				t.Fatal(err)
			}
			if s.Info.BuildMode != mode {
				t.Fatal("build lost its mode")
			}
			for _, m := range s.Info.Materials {
				if m.Block != "minecraft:white_wool" && m.Block != "minecraft:black_terracotta" && m.Block != "minecraft:cobblestone" {
					t.Fatal("export contains an unselected block", m.Block)
				}
			}
			if mode == "2d" {
				if s.Info.Size[1] != 1 || s.Info.SupportBlocks != 0 {
					t.Fatal("flat build has relief or supports")
				}
				for _, block := range s.Blocks {
					if block.Y != 0 {
						t.Fatal("flat block is above the surface")
					}
				}
			}
		})
	}
}

func TestSingleBlockFlatAndStaircaseModes(t *testing.T) {
	opts := DefaultRenderOptions()
	opts.BuildMode = "2d"
	opts.SelectedBlocks = "red_wool"
	opts.MapsX, opts.MapsY = 1, 1
	source := image.NewRGBA(image.Rect(0, 0, 16, 16))
	draw.Draw(source, source.Bounds(), image.NewUniform(color.RGBA{255, 0, 0, 255}), image.Point{}, draw.Src)
	result, err := RenderPreview(source, opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Palette) != 1 {
		t.Fatal("single-block build was not allowed")
	}
	s, err := BuildStructure(result, DefaultBuildOptions())
	if err != nil {
		t.Fatal(err)
	}
	if s.Info.Size[1] != 1 {
		t.Fatal("single block flat build has a height change")
	}
	stairs := structureFixture(1, 5, func(x, z int) int { return 2 })
	stairs.BuildMode = "3d"
	s, err = BuildStructure(stairs, DefaultBuildOptions())
	if err != nil {
		t.Fatal(err)
	}
	if s.Info.Size[1] != 6 {
		t.Fatal("light shades did not create a staircase")
	}
	stairs.BuildMode = "2d"
	if _, err = BuildStructure(stairs, DefaultBuildOptions()); err == nil {
		t.Fatal("flat build accepted light staircase colours")
	}
}

func TestBlockSelectionValidationAndProfileCLI(t *testing.T) {
	for _, value := range []string{"missing_block", "white_wool,", "minecraft:missing", ","} {
		opts := DefaultRenderOptions()
		opts.SelectedBlocks = value
		if err := ValidateOptions(opts); err == nil {
			t.Errorf("accepted %q", value)
		}
	}
	opts := DefaultRenderOptions()
	opts.SelectedBlocks = "minecraft:white_wool,minecraft:black_terracotta"
	opts.BuildMode = "2d"
	opts.Rotation = 270
	opts.FlipHorizontal = true
	opts.FlipVertical = true
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := SaveOptions(path, opts); err != nil {
		t.Fatal(err)
	}
	got, err := LoadOptions(path, DefaultRenderOptions())
	if err != nil || got != opts {
		t.Fatalf("profile changed: %+v, %v", got, err)
	}
	req, err := parseCLI([]string{"-config", path, "-build-mode", "3d", "-blocks", "red_wool", "-rotate", "90", "-flip-horizontal=false"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if req.options.BuildMode != "3d" || req.options.SelectedBlocks != "red_wool" || req.options.Rotation != 90 || req.options.FlipHorizontal || !req.options.FlipVertical {
		t.Fatal("CLI overrides were not applied")
	}
	for _, args := range [][]string{{"-build-mode", "flat"}, {"-rotate", "45"}, {"-build-mode", "2d", "-palette", "custom.json"}} {
		if _, err := parseCLI(args, io.Discard); err == nil {
			t.Fatal("accepted invalid settings", args)
		}
	}
}

func TestEditedFlatMapExportsToNBTAndTiles(t *testing.T) {
	opts := DefaultRenderOptions()
	opts.MapsX, opts.MapsY = 1, 1
	opts.BuildMode = "2d"
	opts.SelectedBlocks = "white_wool,black_wool"
	src := image.NewRGBA(image.Rect(0, 0, 128, 128))
	draw.Draw(src, src.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	result, err := RenderPreview(src, opts)
	if err != nil {
		t.Fatal(err)
	}
	editor := NewPixelEditor(result.Image)
	editor.Line(image.Pt(10, 10), image.Pt(20, 10), 0, palettePixel(result.Palette[1]))
	editor.Commit()
	result.Image = editor.Image
	s, err := BuildStructure(result, DefaultBuildOptions())
	if err != nil {
		t.Fatal(err)
	}
	black := 0
	for _, m := range s.Info.Materials {
		if m.Block == "minecraft:black_wool" {
			black = m.Count
		}
	}
	if black != 11 {
		t.Fatalf("manual edit exported %d black blocks, want 11", black)
	}
	dir := t.TempDir()
	if err := ExportStructure(filepath.Join(dir, "edited.nbt"), s); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "edited.zip")
	if err := ExportStructureTiles(path, result, DefaultBuildOptions()); err != nil {
		t.Fatal(err)
	}
	archive, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	for _, f := range archive.File {
		if f.Name == "layout.json" {
			r, err := f.Open()
			if err != nil {
				t.Fatal(err)
			}
			data, err := io.ReadAll(r)
			r.Close()
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Contains(data, []byte(`"build_mode": "2d"`)) || !strings.Contains(string(data), "minecraft:black_wool") {
				t.Fatal("tile export lost edits or flat mode", string(data))
			}
			return
		}
	}
	t.Fatal("missing tile layout")
}
