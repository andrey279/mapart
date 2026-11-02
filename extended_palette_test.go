package main

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestEveryCatalogVariantCanBuild(t *testing.T) {
	dir := os.Getenv("MAPART_CATALOG_FIXTURES")
	if dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	for id := 0; id < paletteBlockCount; id++ {
		opts := DefaultRenderOptions()
		opts.SelectedBlocks = blockSelectionName(id)
		colors, err := paletteForOptions(opts)
		if err != nil || len(colors) != 3 {
			t.Fatal(opts.SelectedBlocks, len(colors), err)
		}
		img := image.NewRGBA(image.Rect(0, 0, 1, 3))
		for y, p := range colors {
			img.SetRGBA(0, y, palettePixel(p))
		}
		build := DefaultBuildOptions()
		build.Supports = false
		s, err := BuildStructure(&PreviewResult{Image: img, Palette: colors}, build)
		if err != nil {
			t.Fatal(opts.SelectedBlocks, err)
		}
		if dir != "" {
			if err = ExportStructure(filepath.Join(dir, fmt.Sprintf("block_%03d.nbt", id)), s); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestRequestedBlocksAndMinecraftMapColours(t *testing.T) {
	// Values independently extracted from the Java 26.3 block registry.
	cases := map[string]int{
		"white_concrete": 0xffffff, "red_concrete": 0x993333,
		"poplar_planks": 0x999999, "pale_oak_planks": 0xfffcf5,
		"cherry_planks": 0xd1b1a1, "mangrove_planks": 0x993333,
		"bamboo_planks": 0xe5e533, "bamboo_block": 0xe5e533,
		"bamboo_block[axis=x]": 0x007c00, "stripped_bamboo_block": 0xe5e533,
		"deepslate_brick_slab": 0x646464, "deepslate_tiles": 0x646464,
		"blackstone": 0x191919, "polished_blackstone_brick_slab": 0x191919,
		"red_mushroom_block": 0x993333, "brown_mushroom_block": 0x976d4d,
		"mushroom_stem": 0xc7c7c7, "raw_iron_block": 0xd8af93,
		"diamond_ore": 0x707070, "deepslate_diamond_ore": 0x646464,
		"oak_log": 0x8f7748, "oak_log[axis=x]": 0x815631,
	}
	for selection, rgb := range cases {
		t.Run(selection, func(t *testing.T) {
			opts := DefaultRenderOptions()
			opts.SelectedBlocks = selection
			colors, err := paletteForOptions(opts)
			if err != nil || len(colors) != 3 {
				t.Fatal(colors, err)
			}
			for i, mod := range []int{180, 220, 255} {
				want := color.RGBA{uint8((rgb >> 16 & 255) * mod / 255), uint8((rgb >> 8 & 255) * mod / 255), uint8((rgb & 255) * mod / 255), 255}
				if palettePixel(colors[i]) != want {
					t.Fatalf("shade %d = %v, want %v", i, palettePixel(colors[i]), want)
				}
			}
			img := image.NewRGBA(image.Rect(0, 0, 1, 3))
			for y, p := range colors {
				img.SetRGBA(0, y, palettePixel(p))
			}
			build := DefaultBuildOptions()
			build.Supports = false
			s, err := BuildStructure(&PreviewResult{Image: img, Palette: colors}, build)
			if err != nil {
				t.Fatal(err)
			}
			for z := 1; z <= 3; z++ {
				delta := float64(s.Heights[z]-s.Heights[z-1])*4/5 + (float64((z-1)&1)-.5)*.4
				mod := 220
				if delta > .6 {
					mod = 255
				} else if delta < -.6 {
					mod = 180
				}
				want := color.RGBA{uint8((rgb >> 16 & 255) * mod / 255), uint8((rgb >> 8 & 255) * mod / 255), uint8((rgb & 255) * mod / 255), 255}
				if s.MapImage.RGBAAt(0, z-1) != want || s.MapImage.RGBAAt(0, z-1) != img.RGBAAt(0, z-1) {
					t.Fatal("block heights do not reproduce Minecraft shade")
				}
			}
		})
	}
}

func TestFullCatalogSelectionAliasesAndProfile(t *testing.T) {
	opts := DefaultRenderOptions()
	opts.AllBlocks = true
	ids := activeBlockIDs(opts)
	if len(ids) != paletteBlockCount {
		t.Fatal("incomplete full catalogue")
	}
	selected, err := selectedBlockIDs(blockSelection(ids))
	if err != nil || !reflect.DeepEqual(ids, selected) {
		t.Fatal("catalogue selection round trip", err)
	}
	for _, mode := range []string{"2d", "3d"} {
		opts.BuildMode = mode
		colors, err := paletteForOptions(opts)
		if err != nil {
			t.Fatal(err)
		}
		seen := map[color.RGBA]bool{}
		for _, p := range colors {
			if seen[palettePixel(p)] {
				t.Fatal("aliases are counted as extra colours")
			}
			seen[palettePixel(p)] = true
			if mode == "2d" && p.Shade != "Normal" {
				t.Fatal("flat palette has a relief shade")
			}
		}
		want := 180
		if mode == "2d" {
			want = 60
		}
		if len(colors) != want {
			t.Fatalf("map palette has %d colours, want %d", len(colors), want)
		}
	}
	opts.SelectedBlocks = "white_wool,white_concrete"
	colors, err := paletteForOptions(opts)
	if err != nil {
		t.Fatal(err)
	}
	lookup, err := structureColorLookup(&PreviewResult{Image: image.NewRGBA(image.Rect(0, 0, 1, 1)), Palette: colors})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range colors {
		if p.WoolID != 0 || lookup[palettePixel(p)].WoolID != p.WoolID {
			t.Fatal("material preference differs between rendering and export")
		}
	}
	request, err := parseCLI([]string{"-all-blocks", "-blocks", "poplar_planks,deepslate_brick_slab"}, io.Discard)
	if err != nil || !request.options.AllBlocks {
		t.Fatal("full catalogue CLI", err)
	}
	path := filepath.Join(t.TempDir(), "blocks.json")
	if err = SaveOptions(path, request.options); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadOptions(path, DefaultRenderOptions())
	if err != nil || loaded != request.options {
		t.Fatal("new blocks lost in JSON", err)
	}
	preset, _ := PresetOptions("landscape")
	if got := preservePresetAssets(preset, loaded); !got.AllBlocks || got.SelectedBlocks != loaded.SelectedBlocks {
		t.Fatal("preset lost catalogue selection")
	}
	opts.PaletteFile = "custom.json"
	if ValidateOptions(opts) == nil {
		t.Fatal("accepted full catalogue and custom RGB palette")
	}
}

func TestRemovedUnstableBlocksResolveToStableMaterials(t *testing.T) {
	for removed, retained := range map[string]string{"sand": "sandstone", "white_concrete_powder": "white_wool", "white_carpet": "white_wool", "glow_lichen": "verdant_froglight"} {
		opts := DefaultRenderOptions()
		opts.SelectedBlocks = removed
		opts.BuildMode = "2d"
		colors, err := paletteForOptions(opts)
		if err != nil {
			t.Fatal(err)
		}
		img := image.NewRGBA(image.Rect(0, 0, 2, 2))
		for y := 0; y < 2; y++ {
			for x := 0; x < 2; x++ {
				img.SetRGBA(x, y, palettePixel(colors[0]))
			}
		}
		build := DefaultBuildOptions()
		build.Supports = false
		s, err := BuildStructure(&PreviewResult{Image: img, Palette: colors, BuildMode: "2d"}, build)
		if err != nil {
			t.Fatal(err)
		}
		if s.Info.SupportBlocks != 0 || s.Info.Size[1] != 1 {
			t.Fatal("retained stable material received unnecessary supports", removed)
		}
		for _, b := range s.Blocks {
			if b.Z > 0 && s.Palette[b.State] != "minecraft:"+retained {
				t.Fatal("old name exported a removed block", removed, s.Palette[b.State])
			}
		}
	}
}

// A small independent NBT reader checks actual state serialization, including
// compounds inside palette entries. It deliberately does not use nbtWriter.
func readCatalogNBT(r *bytes.Reader, tag byte) (any, error) {
	str := func() (string, error) {
		var n uint16
		if err := binary.Read(r, binary.BigEndian, &n); err != nil {
			return "", err
		}
		b := make([]byte, n)
		_, err := io.ReadFull(r, b)
		return string(b), err
	}
	switch tag {
	case 3:
		var n int32
		err := binary.Read(r, binary.BigEndian, &n)
		return n, err
	case 8:
		return str()
	case 9:
		kind, err := r.ReadByte()
		if err != nil {
			return nil, err
		}
		var n int32
		if err = binary.Read(r, binary.BigEndian, &n); err != nil {
			return nil, err
		}
		out := make([]any, n)
		for i := range out {
			out[i], err = readCatalogNBT(r, kind)
			if err != nil {
				return nil, err
			}
		}
		return out, nil
	case 10:
		out := map[string]any{}
		for {
			kind, err := r.ReadByte()
			if err != nil {
				return nil, err
			}
			if kind == 0 {
				return out, nil
			}
			key, err := str()
			if err != nil {
				return nil, err
			}
			value, err := readCatalogNBT(r, kind)
			if err != nil {
				return nil, err
			}
			out[key] = value
		}
	}
	return nil, fmt.Errorf("unexpected tag %d", tag)
}

func TestNewBlocksNBTStatesAndVersion(t *testing.T) {
	for _, name := range []string{"bamboo_block[axis=x]", "deepslate_brick_slab", "pale_oak_planks", "mushroom_stem"} {
		opts := DefaultRenderOptions()
		opts.SelectedBlocks = name
		opts.BuildMode = "2d"
		colors, err := paletteForOptions(opts)
		if err != nil {
			t.Fatal(err)
		}
		img := image.NewRGBA(image.Rect(0, 0, 1, 1))
		img.SetRGBA(0, 0, palettePixel(colors[0]))
		result := &PreviewResult{Image: img, Palette: colors, BuildMode: "2d"}
		s, err := BuildStructure(result, DefaultBuildOptions())
		if err != nil {
			t.Fatal(err)
		}
		var buffer bytes.Buffer
		if err = WriteStructureNBT(&buffer, s); err != nil {
			t.Fatal(err)
		}
		gz, err := gzip.NewReader(&buffer)
		if err != nil {
			t.Fatal(err)
		}
		raw, err := io.ReadAll(gz)
		gz.Close()
		if err != nil {
			t.Fatal(err)
		}
		reader := bytes.NewReader(raw)
		rootTag, _ := reader.ReadByte()
		var rootName uint16
		binary.Read(reader, binary.BigEndian, &rootName)
		if rootName != 0 {
			t.Fatal("unexpected root name")
		}
		parsed, err := readCatalogNBT(reader, rootTag)
		if err != nil {
			t.Fatal(err)
		}
		root := parsed.(map[string]any)
		if root["DataVersion"] != int32(5023) {
			t.Fatal("wrong target version")
		}
		found := false
		for _, entry := range root["palette"].([]any) {
			p := entry.(map[string]any)
			if p["Name"] != "minecraft:"+strings.Split(name, "[")[0] {
				continue
			}
			found = true
			props, _ := p["Properties"].(map[string]any)
			if name == "bamboo_block[axis=x]" && props["axis"] != "x" {
				t.Fatal("log orientation lost")
			}
			if name == "deepslate_brick_slab" && (props["type"] != "top" || props["waterlogged"] != "false") {
				t.Fatal("slab state lost")
			}
			if name == "mushroom_stem" && props["up"] != "true" {
				t.Fatal("mushroom faces lost")
			}
		}
		if !found {
			t.Fatal("selected material absent from NBT")
		}
		if name == "pale_oak_planks" {
			old := DefaultBuildOptions()
			old.DataVersion = 3465
			if _, err = BuildStructure(result, old); err == nil {
				t.Fatal("new wood exported as Java 1.20.1")
			}
		}
	}
}
