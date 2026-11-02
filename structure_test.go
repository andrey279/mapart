package main

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"fmt"
	"image"
	"image/color"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func structureFixture(w, h int, pick func(x, z int) int) *PreviewResult {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for z := 0; z < h; z++ {
		for x := 0; x < w; x++ {
			p := palette[pick(x, z)]
			img.SetRGBA(x, z, color.RGBA{uint8(math.Round(p.R)), uint8(math.Round(p.G)), uint8(math.Round(p.B)), 255})
		}
	}
	return &PreviewResult{Image: img, Palette: palette}
}

func TestStructurePreservesJavaShadingForEveryToneSequence(t *testing.T) {
	// Exercise all light/normal/dark arrangements of length 7, including
	// plateaus between direction changes. Check the game's actual formula.
	for code := 0; code < 2187; code++ {
		n := code
		var shades [7]int
		for i := range shades {
			shades[i] = n % 3
			n /= 3
		}
		r := structureFixture(1, 7, func(x, z int) int { return shades[z] })
		opts := DefaultBuildOptions()
		opts.Supports = false
		s, err := BuildStructure(r, opts)
		if err != nil {
			t.Fatal(err)
		}
		for z, want := range shades {
			delta := s.Heights[z+1] - s.Heights[z]
			for parity := 0; parity < 2; parity++ {
				value := float64(delta)*4.0/5.0 + (float64(parity)-.5)*.4
				got := 1
				if value > .6 {
					got = 2
				} else if value < -.6 {
					got = 0
				}
				if got != want {
					t.Fatalf("sequence %v z=%d heights=%v parity=%d got%d want%d", shades, z, s.Heights, parity, got, want)
				}
			}
		}
	}
}

func TestStructureBlocksMaterialsAndIntegerMapColors(t *testing.T) {
	r := structureFixture(48, 9, func(x, z int) int { return (x + z*5) % 48 })
	s, err := BuildStructure(r, DefaultBuildOptions())
	if err != nil {
		t.Fatal(err)
	}
	seen := map[[3]int]bool{}
	counts := map[string]int{}
	for _, b := range s.Blocks {
		pos := [3]int{b.X, b.Y, b.Z}
		if seen[pos] {
			t.Fatal("duplicate block", pos)
		}
		seen[pos] = true
		for i, v := range pos {
			if v < 0 || v >= s.Info.Size[i] {
				t.Fatal("out of bounds", pos, s.Info.Size)
			}
		}
		if b.State < 0 || b.State >= len(s.Palette) {
			t.Fatal("invalid palette index")
		}
		counts[s.Palette[b.State]]++
		if b.Y == s.Heights[b.Z*48+b.X] && b.Z > 0 {
			p := palette[(b.X+(b.Z-1)*5)%48]
			if s.Palette[b.State] != "minecraft:"+woolNames[p.WoolID]+"_wool" {
				t.Fatal("wrong wool")
			}
			modifier := []int{180, 220, 255}[(b.X+(b.Z-1)*5)%3]
			base := baseColors[p.WoolID]
			want := color.RGBA{uint8(int(base[0]) * modifier / 255), uint8(int(base[1]) * modifier / 255), uint8(int(base[2]) * modifier / 255), 255}
			if s.MapImage.RGBAAt(b.X, b.Z-1) != want {
				t.Fatal("in-game preview wrong")
			}
		} else if s.Palette[b.State] != "minecraft:cobblestone" {
			t.Fatal("unexpected support material")
		}
	}
	for _, m := range s.Info.Materials {
		if counts[m.Block] != m.Count || m.Stacks*64+m.Remainder != m.Count {
			t.Fatal("wrong material count")
		}
	}
	if s.Info.TotalBlocks != s.Info.SurfaceBlocks+s.Info.NorthBlocks+s.Info.SupportBlocks {
		t.Fatal("wrong total")
	}
	// Every north/south step is physically connected by a support column.
	for z := 1; z <= 9; z++ {
		for x := 0; x < 48; x++ {
			a, b := s.Heights[(z-1)*48+x], s.Heights[z*48+x]
			highZ := z
			if a > b {
				highZ = z - 1
			}
			for y := min(a, b); y <= max(a, b); y++ {
				if !seen[[3]int{x, y, highZ}] {
					t.Fatal("disconnected step")
				}
			}
		}
	}
}

func TestStructureHeightLimitAndIndependentTiles(t *testing.T) {
	r := structureFixture(128, 256, func(x, z int) int { return 2 })
	opts := DefaultBuildOptions()
	opts.MaxHeight = 200
	if _, err := BuildStructure(r, opts); err == nil {
		t.Fatal("accepted impossible height")
	}
	path := filepath.Join(t.TempDir(), "tiles.zip")
	if err := ExportStructureTiles(path, r, opts); err != nil {
		t.Fatal(err)
	}
	z, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	nbtCount := 0
	for _, f := range z.File {
		if strings.HasSuffix(f.Name, ".nbt") {
			nbtCount++
			reader, err := f.Open()
			if err != nil {
				t.Fatal(err)
			}
			g, err := gzip.NewReader(reader)
			if err != nil {
				t.Fatal(err)
			}
			raw, err := io.ReadAll(g)
			g.Close()
			reader.Close()
			if err != nil || len(raw) < 100 {
				t.Fatal("broken NBT member")
			}
		}
	}
	if nbtCount != 2 {
		t.Fatal("wrong number of maps", nbtCount)
	}
}

func TestStructureRejectsCustomPaletteAndInvalidOptions(t *testing.T) {
	r := structureFixture(1, 1, func(x, z int) int { return 2 })
	r.Palette = []MapColor{{Shade: "Custom", WoolID: 0}}
	if _, err := BuildStructure(r, DefaultBuildOptions()); err == nil {
		t.Fatal("custom color has no block mapping")
	}
	for _, opts := range []BuildOptions{{true, 1, 3465}, {true, 5000, 3465}, {true, 384, 0}} {
		if opts.Validate() == nil {
			t.Fatal("accepted invalid options")
		}
	}
}

func TestNBTExportArtifactsAndAtomicFailures(t *testing.T) {
	r := structureFixture(8, 8, func(x, z int) int { return (x + z) % 48 })
	s, err := BuildStructure(r, DefaultBuildOptions())
	if err != nil {
		t.Fatal(err)
	}
	var first, second bytes.Buffer
	if err = WriteStructureNBT(&first, s); err != nil {
		t.Fatal(err)
	}
	WriteStructureNBT(&second, s)
	if !bytes.Equal(first.Bytes(), second.Bytes()) {
		t.Fatal("NBT is not deterministic")
	}
	g, err := gzip.NewReader(&first)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(g)
	g.Close()
	if err != nil {
		t.Fatal(err)
	}
	if raw[0] != 10 || raw[len(raw)-1] != 0 {
		t.Fatal("invalid root compound")
	}
	for _, key := range []string{"DataVersion", "size", "palette", "blocks", "entities", "minecraft:white_wool"} {
		if !bytes.Contains(raw, []byte(key)) {
			t.Fatal("missing NBT key", key)
		}
	}
	path := filepath.Join(t.TempDir(), "nested", "art.nbt")
	if err = ExportStructure(path, s); err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{".nbt", "_materials.csv", "_build.json", "_map.png"} {
		if _, err = os.Stat(strings.TrimSuffix(path, ".nbt") + suffix); err != nil {
			t.Fatal(err)
		}
	}
	before, _ := os.ReadFile(path)
	err = atomicOutput(path, func(w io.Writer) error { w.Write([]byte("broken")); return fmt.Errorf("write interrupted") })
	if err == nil {
		t.Fatal("write failure lost")
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(before, after) {
		t.Fatal("failed write replaced existing file")
	}
	if err = ExportStructure(path, s); err != nil {
		t.Fatal("cannot replace existing output", err)
	}
}

func TestCLIRejectsInvalidStructureCombinations(t *testing.T) {
	for _, args := range [][]string{{"-nbt", "art.nbt"}, {"-i", "missing.png", "-nbt", "art.zip"}, {"-i", "missing.png", "-nbt-tiles", "art.nbt"}, {"-i", "missing.png", "-nbt", "art.nbt", "-compare", "all"}, {"-i", "missing.png", "-nbt", "art.nbt", "-palette", "custom.json"}} {
		if err := runCLI(args); err == nil || strings.Contains(err.Error(), "missing.png") {
			t.Fatalf("invalid NBT combination reached image loading: %v: %v", args, err)
		}
	}
}
