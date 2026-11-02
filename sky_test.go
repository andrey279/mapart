package main

import (
	"image"
	"image/color"
	"testing"
)

func TestLightSkyWithDetailedBorders(t *testing.T) {
	const width, height = 192, 128
	src := image.NewRGBA(image.Rect(0, 0, width, height))
	frame := []color.RGBA{{55, 85, 125, 255}, {165, 65, 135, 255}, {65, 145, 120, 255}}
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			// Colored trees and ground cover most of the image border, so
			// the old dominant-border-color detector cannot find this sky.
			p := frame[(x/18+y/14)%len(frame)]
			if x >= 36 && x < 156 && y < 72+(x/24)%7 {
				// A gently shaded, slightly cool sky rather than pure white.
				v := uint8(245 - y/6)
				p = color.RGBA{v - 2, v, v + 3, 255}
			}
			if x >= 82 && x < 108 && y >= 28 && y < 44 {
				p = color.RGBA{235, 85, 165, 255} // Pink leaves in the sky.
			}
			if x >= 118 && x < 124 && y >= 24 && y < 48 {
				p = color.RGBA{65, 55, 75, 255} // A contrasting branch.
			}
			if x >= 84 && x < 110 && y >= 94 && y < 118 {
				p = color.RGBA{242, 244, 247, 255} // Separate pale foreground object.
			}
			src.SetRGBA(x, y, p)
		}
	}

	mask, _ := detectBackground(src)
	for _, point := range []image.Point{{60, 12}, {130, 16}, {65, 54}, {134, 58}} {
		if !mask[point.Y*width+point.X] {
			t.Errorf("smooth light sky at %v still dithers", point)
		}
	}
	for name, point := range map[string]image.Point{
		"pink leaves":              {95, 35},
		"dark branch":              {121, 36},
		"separate pale foreground": {96, 104},
		"colored scenery":          {18, 60},
	} {
		if mask[point.Y*width+point.X] {
			t.Errorf("%s at %v incorrectly lost dithering", name, point)
		}
	}
}

func TestBrightTextureAtTopIsNotSmoothSky(t *testing.T) {
	const width, height = 192, 128
	src := image.NewRGBA(image.Rect(0, 0, width, height))
	frame := []color.RGBA{{50, 80, 125, 255}, {165, 65, 135, 255}, {65, 145, 120, 255}}
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			p := frame[(x/18+y/14)%len(frame)]
			if x >= 36 && x < 156 && y < 72 {
				// Fine light-and-shadow detail has a pale average color,
				// but must retain dithering even when it touches the top.
				p = color.RGBA{252, 253, 255, 255}
				if (x/2+y/2)%2 != 0 {
					p = color.RGBA{175, 185, 195, 255}
				}
			}
			src.SetRGBA(x, y, p)
		}
	}

	mask, _ := detectBackground(src)
	selected := 0
	for y := 8; y < 64; y++ {
		for x := 44; x < 148; x++ {
			if mask[y*width+x] {
				selected++
			}
		}
	}
	if selected != 0 {
		t.Fatalf("bright textured area incorrectly contains %d non-dithered pixels", selected)
	}
}
