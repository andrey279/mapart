package main

import (
	"image"
	"image/color"
	"math"
	"testing"
)

func TestBackgroundPreservesDistinctToneAcrossSmoothTransition(t *testing.T) {
	const size = 128
	src := image.NewRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			// A softly lit blue cloud inside a darker, otherwise uniform
			// backdrop. Its edge has no sharp jump, but its central tone
			// differs enough that it must retain shading and dithering.
			radius := math.Hypot(float64(x-size/2), float64(y-size/2))
			light := math.Max(0, math.Min(1, (48-radius)/32))
			src.SetRGBA(x, y, color.RGBA{
				R: uint8(math.Round(2 + 10*light)),
				G: uint8(math.Round(4 + 20*light)),
				B: uint8(math.Round(72 + 60*light)),
				A: 255,
			})
		}
	}

	background, _ := detectBackground(src)
	if !background[8*size+8] {
		t.Error("uniform dark-blue backdrop should suppress dithering")
	}
	if background[(size/2)*size+size/2] {
		t.Error("smooth color change expanded the flat backdrop into a distinct lighter tone")
	}
}
