package main

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"sort"
)

// QuantizeImage maps an already scaled, opaque image into colors. The caller
// owns the palette and masks; this function never changes them or writes files.
// Threshold is always measured in CIEDE2000, including the RGB matching mode.
// A nil background mask means every pixel may be dithered.
func QuantizeImage(scaled *image.RGBA, background []bool, smoothed []color.RGBA, opts RenderOptions, colors []MapColor) (*image.RGBA, error) {
	if scaled == nil || scaled.Bounds().Empty() {
		return nil, fmt.Errorf("quantization requires a nonempty image")
	}
	if opts.Dither != "fs" && opts.Dither != "ordered" && opts.Dither != "none" {
		return nil, fmt.Errorf("unknown dither mode %q", opts.Dither)
	}
	if opts.ColorMetric != "lab" && opts.ColorMetric != "rgb" {
		return nil, fmt.Errorf("unknown color metric %q", opts.ColorMetric)
	}
	if math.IsNaN(opts.DitherStrength) || math.IsInf(opts.DitherStrength, 0) || opts.DitherStrength < 0 || opts.DitherStrength > 1 {
		return nil, fmt.Errorf("dither strength must be between 0 and 1")
	}
	if math.IsNaN(opts.Threshold) || math.IsInf(opts.Threshold, 0) || opts.Threshold < 0 {
		return nil, fmt.Errorf("dither threshold must be finite and nonnegative")
	}
	if len(colors) == 0 {
		return nil, fmt.Errorf("quantization requires at least one palette color")
	}
	if opts.Dither == "ordered" && len(colors) > 256 {
		return nil, fmt.Errorf("ordered dithering supports at most 256 palette colors")
	}
	for _, c := range colors {
		for _, v := range [...]float64{c.R, c.G, c.B} {
			if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > 255 {
				return nil, fmt.Errorf("palette RGB values must be finite and between 0 and 255")
			}
		}
		for _, v := range [...]float64{c.L, c.A, c.B_lab} {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				return nil, fmt.Errorf("palette Lab values must be finite")
			}
		}
	}
	bounds := scaled.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	count := width * height
	if background != nil && len(background) != count {
		return nil, fmt.Errorf("background mask dimensions do not match image")
	}
	for _, masked := range background {
		if masked && len(smoothed) != count {
			return nil, fmt.Errorf("masked pixels require a full smoothed image")
		}
	}
	isBackground := func(i int) bool { return background != nil && background[i] }
	buffer := make([][3]float64, count)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			p := scaled.RGBAAt(bounds.Min.X+x, bounds.Min.Y+y)
			buffer[y*width+x] = [3]float64{float64(p.R), float64(p.G), float64(p.B)}
		}
	}
	out := image.NewRGBA(bounds)
	var orderedCache map[uint32][64]uint8
	if opts.Dither == "ordered" {
		orderedCache = make(map[uint32][64]uint8)
	}
	for y := 0; y < height; y++ {
		for step := 0; step < width; step++ {
			x, direction := step, 1
			if y%2 == 1 {
				x, direction = width-1-step, -1
			}
			i := y*width + x
			c := buffer[i]
			masked := isBackground(i)
			if masked {
				p := smoothed[i]
				c = [3]float64{float64(p.R), float64(p.G), float64(p.B)}
			} else if opts.CleanHaze && opts.Dither == "fs" {
				p := scaled.RGBAAt(bounds.Min.X+x, bounds.Min.Y+y)
				c = constrainChromaError([3]float64{float64(p.R), float64(p.G), float64(p.B)}, c)
			}
			unclamped := c
			for channel := range c {
				c[channel] = math.Max(0, math.Min(255, c[channel]))
			}
			best, deltaE := nearestQuantizeColor(c, colors, opts.ColorMetric)
			canDither := !masked && opts.DitherStrength > 0 && deltaE > opts.Threshold
			if canDither && opts.Dither == "ordered" {
				key := uint32(c[0])<<16 | uint32(c[1])<<8 | uint32(c[2])
				plan, found := orderedCache[key]
				if !found {
					plan = orderedQuantizePlan(c, best, colors, opts.DitherStrength)
					if len(orderedCache) < 65536 {
						orderedCache[key] = plan
					}
				}
				best = int(plan[quantizeBayer8[y&7][x&7]])
			}
			chosen := colors[best]
			if canDither && opts.Dither == "fs" {
				// Retain the original clamped arithmetic for Lab mode. RGB mode
				// preserves residual beyond gamut clipping, which is necessary to
				// reproduce pale mixtures at full strength. A one-channel residual
				// cannot exceed one full RGB range, even for unreachable colors.
				errSource := c
				if opts.ColorMetric == "rgb" {
					errSource = unclamped
				}
				err := [3]float64{errSource[0] - chosen.R, errSource[1] - chosen.G, errSource[2] - chosen.B}
				for channel := range err {
					if opts.ColorMetric == "rgb" {
						err[channel] = math.Max(-255, math.Min(255, err[channel]))
					}
					err[channel] *= opts.DitherStrength
				}
				distributeError := func(dx, dy int, weight float64) {
					nx, ny := x+dx, y+dy
					if nx >= 0 && nx < width && ny >= 0 && ny < height && !isBackground(ny*width+nx) {
						next := &buffer[ny*width+nx]
						next[0] += err[0] * weight
						next[1] += err[1] * weight
						next[2] += err[2] * weight
					}
				}
				distributeError(direction, 0, 7.0/16.0)
				distributeError(-direction, 1, 3.0/16.0)
				distributeError(0, 1, 5.0/16.0)
				distributeError(direction, 1, 1.0/16.0)
			}
			out.SetRGBA(bounds.Min.X+x, bounds.Min.Y+y, color.RGBA{uint8(math.Round(chosen.R)), uint8(math.Round(chosen.G)), uint8(math.Round(chosen.B)), 255})
		}
	}
	return out, nil
}

func nearestQuantizeColor(c [3]float64, colors []MapColor, metric string) (int, float64) {
	l, a, b := rgbToLab(c[0], c[1], c[2])
	best, bestDistance := 0, math.MaxFloat64
	for i, p := range colors {
		var distance float64
		if metric == "rgb" {
			dr, dg, db := c[0]-p.R, c[1]-p.G, c[2]-p.B
			distance = dr*dr + dg*dg + db*db
		} else {
			distance = ciede2000(l, a, b, p.L, p.A, p.B_lab)
		}
		if distance < bestDistance {
			best, bestDistance = i, distance
		}
	}
	if metric == "lab" {
		return best, bestDistance
	}
	p := colors[best]
	return best, ciede2000(l, a, b, p.L, p.A, p.B_lab)
}

// Build a 64-sample palette mixture, then distribute it with Bayer ordering.
// A mixture restricted to two colors jumps abruptly between complementary
// pairs on smooth beige gradients. Accumulating RGB residual lets several
// nearby colors share the tile and preserves its average without those bands.
// Metric selects the anchor; strength interpolates from it to the source.
func orderedQuantizePlan(c [3]float64, first int, colors []MapColor, strength float64) [64]uint8 {
	p := colors[first]
	target := [3]float64{p.R + strength*(c[0]-p.R), p.G + strength*(c[1]-p.G), p.B + strength*(c[2]-p.B)}
	var plan [64]uint8
	var residual [3]float64
	for step := range plan {
		var corrected [3]float64
		for ch := range corrected {
			corrected[ch] = target[ch] + residual[ch]
		}
		best, distance := 0, math.MaxFloat64
		for i, q := range colors {
			r, g, b := corrected[0]-q.R, corrected[1]-q.G, corrected[2]-q.B
			d := r*r + g*g + b*b
			if d < distance {
				best, distance = i, d
			}
		}
		plan[step] = uint8(best)
		q := colors[best]
		for ch, value := range [3]float64{q.R, q.G, q.B} {
			residual[ch] += target[ch] - value
		}
	}
	sort.Slice(plan[:], func(i, j int) bool {
		a, b := colors[plan[i]], colors[plan[j]]
		if a.L == b.L {
			return plan[i] < plan[j]
		}
		return a.L < b.L
	})
	return plan
}

var quantizeBayer8 = [8][8]uint8{
	{0, 48, 12, 60, 3, 51, 15, 63},
	{32, 16, 44, 28, 35, 19, 47, 31},
	{8, 56, 4, 52, 11, 59, 7, 55},
	{40, 24, 36, 20, 43, 27, 39, 23},
	{2, 50, 14, 62, 1, 49, 13, 61},
	{34, 18, 46, 30, 33, 17, 45, 29},
	{10, 58, 6, 54, 9, 57, 5, 53},
	{42, 26, 38, 22, 41, 25, 37, 21},
}
