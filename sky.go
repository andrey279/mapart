package main

import (
	"image"
	"image/color"
)

// suppressSkyDithering adds large, smooth, pale regions connected to the top
// edge to the no-dither mask. It runs independently of the plain-background
// detector, since a landscape usually has many colors along its other edges.
func suppressSkyDithering(img *image.RGBA, background []bool, smooth []color.RGBA) {
	width, height := img.Bounds().Dx(), img.Bounds().Dy()
	if width == 0 || height == 0 {
		return
	}
	bright := func(p color.RGBA) bool {
		lo := min(p.R, p.G, p.B)
		hi := max(p.R, p.G, p.B)
		luminance := (299*int(p.R) + 587*int(p.G) + 114*int(p.B)) / 1000
		return lo >= 224 && luminance >= 233 && int(hi)-int(lo) <= 35
	}
	// Cache suitability lazily: most of a scene is never reached from the sky.
	// Check texture in the source, as smoothing can hide fine leaves or stars.
	state := make([]uint8, width*height) // 0: unknown, 1: blocked, 2: smooth sky
	eligible := func(i int) bool {
		if state[i] != 0 {
			return state[i] == 2
		}
		state[i] = 1
		if !bright(smooth[i]) {
			return false
		}
		x, y := i%width, i/width
		var sum, squared [3]int
		count := 0
		for ny := max(0, y-2); ny <= min(height-1, y+2); ny++ {
			for nx := max(0, x-2); nx <= min(width-1, x+2); nx++ {
				p := img.RGBAAt(img.Rect.Min.X+nx, img.Rect.Min.Y+ny)
				for c, v := range [3]int{int(p.R), int(p.G), int(p.B)} {
					sum[c] += v
					squared[c] += v * v
				}
				count++
			}
		}
		for c := range sum {
			// Channel standard deviation must stay below seven RGB levels.
			if squared[c]*count-sum[c]*sum[c] > 49*count*count {
				return false
			}
		}
		state[i] = 2
		return true
	}
	visited := make([]bool, width*height)
	minArea := max(32, width*height/500)
	for x := 0; x < width; x++ {
		if visited[x] || !eligible(x) {
			continue
		}
		component := []int{x}
		visited[x] = true
		topPixels := 0
		for head := 0; head < len(component); head++ {
			i := component[head]
			cx, cy := i%width, i/width
			if cy == 0 {
				topPixels++
			}
			for _, n := range [][2]int{{cx - 1, cy}, {cx + 1, cy}, {cx, cy - 1}, {cx, cy + 1}} {
				if n[0] < 0 || n[0] >= width || n[1] < 0 || n[1] >= height {
					continue
				}
				j := n[1]*width + n[0]
				if visited[j] || !eligible(j) {
					continue
				}
				visited[j] = true
				component = append(component, j)
			}
		}
		if len(component) >= minArea && topPixels >= max(3, width/100) {
			for _, i := range component {
				background[i] = true
			}
		}
	}
}
