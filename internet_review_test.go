//go:build internetreview

package main

import (
	"encoding/json"
	"image"
	"image/color"
	"image/draw"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Optional inspection of real downloaded fixtures; normal unit tests do not
// depend on network images. No-dither overlays explain where detail was lost.
func TestInspectInternetBackgroundMasks(t *testing.T) {
	root := "verification/internet_batch"
	paths, err := filepath.Glob(filepath.Join(root, "originals", "*.png"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("render the downloaded fixtures first: %v", err)
	}
	outDir := filepath.Join(root, "masks")
	if err := os.MkdirAll(outDir, 0755); err != nil {
		t.Fatal(err)
	}
	counts := map[string]map[string]int{}
	for _, path := range paths {
		f, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		decoded, _, err := image.Decode(f)
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
		img := image.NewRGBA(decoded.Bounds())
		draw.Draw(img, img.Bounds(), decoded, decoded.Bounds().Min, draw.Src)
		mask, smooth := detectBackground(img)
		sky := make([]bool, len(mask))
		suppressSkyDithering(img, sky, smooth)
		binary := image.NewGray(img.Bounds())
		overlay := image.NewRGBA(img.Bounds())
		selected, skySelected := 0, 0
		for i, enabled := range mask {
			x, y := i%img.Bounds().Dx(), i/img.Bounds().Dx()
			p := img.RGBAAt(x, y)
			if sky[i] {
				skySelected++
			}
			if enabled {
				selected++
				binary.SetGray(x, y, color.Gray{255})
				p = color.RGBA{p.R / 2, uint8((int(p.G) + 255) / 2), p.B / 2, 255}
			}
			overlay.SetRGBA(x, y, p)
		}
		name := strings.TrimSuffix(filepath.Base(path), ".png")
		counts[name] = map[string]int{"total_pixels": len(mask), "no_dither_pixels": selected, "sky_pixels": skySelected}
		if err := savePNG(filepath.Join(outDir, name+".png"), binary); err != nil {
			t.Fatal(err)
		}
		if err := savePNG(filepath.Join(outDir, name+"_overlay.png"), overlay); err != nil {
			t.Fatal(err)
		}
	}
	data, err := json.MarshalIndent(counts, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "counts.json"), data, 0644); err != nil {
		t.Fatal(err)
	}
}
