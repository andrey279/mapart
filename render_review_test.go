//go:build renderreview

package main

import (
	"image"
	"os"
	"path/filepath"
	"testing"
)

func TestRenderChromaReview(t *testing.T) {
	if err := os.MkdirAll("verification/chroma_fix/after", 0755); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, path string
		x, y       int
	}{
		{"cherry_blossom", "anime_gallery/sources/cherry_blossom.png", 5, 3},
		{"fantasy_vista", "anime_gallery/sources/fantasy_vista.png", 3, 4},
		{"sylva", "anime_gallery/sources/sylva.png", 3, 4},
		{"spring_figure", "anime_gallery/sources/spring_figure.png", 4, 4},
		{"sakaki_ness", "anime_gallery/sources/sakaki_ness.png", 4, 5},
		{"neon_city", "anime_gallery/sources/neon_city.png", 3, 5},
		{"source", "source.png", 5, 3},
		{"preprocessed", "preprocessed.png", 5, 3},
		{"input_2", "input/2.png", 2, 3},
		{"input_3", "input/3.png", 2, 3},
		{"input_jpg", "input/a17643747d88449264ca903ec4a094ce.jpg", 2, 3},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			f, err := os.Open(c.path)
			if err != nil {
				t.Fatal(err)
			}
			img, _, err := image.Decode(f)
			f.Close()
			if err != nil {
				t.Fatal(err)
			}
			out := filepath.Join("verification/chroma_fix/after", c.name+".png")
			if err := processImage(img, out, c.x, c.y, 5); err != nil {
				t.Fatal(err)
			}
		})
	}
}
