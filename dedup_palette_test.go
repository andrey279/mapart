package main

import (
	"encoding/json"
	"fmt"
	"image"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestCatalogContainsOneMaterialPerMinecraftColour(t *testing.T) {
	if paletteBlockCount != 60 {
		t.Fatalf("active catalogue has %d materials, want 60", paletteBlockCount)
	}
	seen := map[[3]float64]string{}
	for id := 0; id < paletteBlockCount; id++ {
		name, rgb, ok := paletteBlock(id)
		if !ok {
			t.Fatal("invalid retained material", id)
		}
		if previous, duplicate := seen[rgb]; duplicate {
			t.Fatal("duplicate Minecraft map colour", previous, name)
		}
		seen[rgb] = name
		if blockNeedsSupport(id) {
			t.Fatal("unstable duplicate retained instead of a stable equivalent", name)
		}
	}
	for _, group := range blockGroups {
		count := 0
		for id := 0; id < paletteBlockCount; id++ {
			if blockGroup(id) == group {
				count++
			}
		}
		if count == 0 {
			t.Fatal("empty material group remains visible", group)
		}
	}
}

func TestDeduplicationPreservesPaletteColoursAndOrder(t *testing.T) {
	raw, err := os.ReadFile("verification/block_catalog/before_dedup_palette.json")
	if err != nil {
		t.Fatal(err)
	}
	var before []string
	if err = json.Unmarshal(raw, &before); err != nil {
		t.Fatal(err)
	}
	opts := DefaultRenderOptions()
	opts.AllBlocks = true
	colors, err := paletteForOptions(opts)
	if err != nil {
		t.Fatal(err)
	}
	after := make([]string, len(colors))
	for i, p := range colors {
		c := palettePixel(p)
		after[i] = fmt.Sprintf("#%02X%02X%02X", c.R, c.G, c.B)
	}
	if len(after) != 180 || !reflect.DeepEqual(before, after) {
		t.Fatal("removing material aliases changed colours or quantization tie-breaking")
	}
}

func TestRemovedMaterialNamesMigrateWithoutReintroducingDuplicates(t *testing.T) {
	for removed, retained := range map[string]string{"white_concrete": "white_wool", "poplar_planks": "light_gray_wool", "blackstone": "black_wool", "red_mushroom_block": "red_wool", "brown_mushroom_block": "dirt", "bamboo_planks": "yellow_wool"} {
		ids, err := selectedBlockIDs(removed + "," + retained)
		if err != nil {
			t.Fatal(err)
		}
		if len(ids) != 1 || blockSelection(ids) != "minecraft:"+retained {
			t.Fatal("removed duplicate survived profile migration", removed, ids)
		}
		for id := 0; id < paletteBlockCount; id++ {
			if blockSelectionName(id) == "minecraft:"+removed {
				t.Fatal("removed material remains active", removed)
			}
		}
		opts := DefaultRenderOptions()
		opts.SelectedBlocks = removed
		opts.BuildMode = "2d"
		colors, err := paletteForOptions(opts)
		if err != nil {
			t.Fatal(err)
		}
		img := image.NewRGBA(image.Rect(0, 0, 1, 1))
		img.SetRGBA(0, 0, palettePixel(colors[0]))
		s, err := BuildStructure(&PreviewResult{Image: img, Palette: colors, BuildMode: "2d"}, DefaultBuildOptions())
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range s.Palette {
			if strings.TrimPrefix(name, "minecraft:") == removed {
				t.Fatal("NBT exported a removed alias", removed)
			}
		}
	}
}

func TestEveryRemovedAliasPreservesItsVerifiedGameColour(t *testing.T) {
	raw, err := os.ReadFile("verification/block_catalog/blocks_26_3.json")
	if err != nil {
		t.Fatal(err)
	}
	var old []struct {
		Selection string `json:"selection"`
		RGB       int    `json:"rgb"`
	}
	if err = json.Unmarshal(raw, &old); err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, row := range old {
		if _, removed := removedBlockAliases[row.Selection]; !removed {
			continue
		}
		ids, err := selectedBlockIDs(row.Selection)
		if err != nil || len(ids) != 1 {
			t.Fatal("invalid migration", row.Selection, err)
		}
		for id := range ids {
			_, rgb, _ := paletteBlock(id)
			actual := int(rgb[0])<<16 | int(rgb[1])<<8 | int(rgb[2])
			if actual != row.RGB {
				t.Fatal("migration changed map colour", row.Selection, blockSelectionName(id))
			}
		}
		checked++
	}
	if checked != 625 || checked != len(removedBlockAliases) {
		t.Fatal("incomplete alias migration coverage", checked)
	}
}
