package main

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"image"
	"image/color"
	"math"
	"testing"
)

func quantizeRegressionSource() *image.RGBA {
	src := image.NewRGBA(image.Rect(0, 0, 128, 128))
	for y := 0; y < 128; y++ {
		for x := 0; x < 128; x++ {
			p := color.RGBA{uint8((x*9 + y*3) % 256), uint8((x*2 + y*7) % 256), uint8((x*5 + y*11) % 256), 255}
			if y < 24 {
				p = color.RGBA{uint8(243 + x/24), uint8(240 + x/32), 245, 255}
			}
			if x >= 32 && x < 96 && y >= 40 && y < 80 {
				p = color.RGBA{250, uint8(217 + y/8), 202, 255}
			}
			src.SetRGBA(x, y, p)
		}
	}
	return src
}

func TestQuantizeDefaultMatchesLegacySnapshot(t *testing.T) {
	// Captured from processImage before extracting the quantizer, with the
	// 45-distance background detector, sky cleanup enabled, haze cleanup off.
	// This fixes the actual previous output, rather than duplicating its code.
	src := quantizeRegressionSource()
	mask, smooth := detectBackground(src)
	im := mustQuantize(t, src, mask, smooth, DefaultRenderOptions(), palette)
	buf := make([]byte, 0, 128*128*3)
	for y := 0; y < 128; y++ {
		for x := 0; x < 128; x++ {
			p := im.RGBAAt(x, y)
			buf = append(buf, p.R, p.G, p.B)
		}
	}
	want := "8675af5ffb652c619f42317b39067f8ec842396d198a417cb8a808c5a3de50d9"
	if got := fmt.Sprintf("%x", sha256.Sum256(buf)); got != want {
		t.Fatalf("default RGB pixels changed: got %s, want %s", got, want)
	}
}

func mustQuantize(t *testing.T, src *image.RGBA, mask []bool, smooth []color.RGBA, opts RenderOptions, colors []MapColor) *image.RGBA {
	t.Helper()
	got, err := QuantizeImage(src, mask, smooth, opts, colors)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func testQuantizeColor(r, g, b float64) MapColor {
	l, a, labB := rgbToLab(r, g, b)
	return MapColor{R: r, G: g, B: b, L: l, A: a, B_lab: labB}
}

func TestQuantizeUsesOnlySuppliedPaletteAndIsDeterministic(t *testing.T) {
	src := image.NewRGBA(image.Rect(7, 11, 39, 35))
	for y := 11; y < 35; y++ {
		for x := 7; x < 39; x++ {
			src.SetRGBA(x, y, color.RGBA{uint8(x * 5), uint8(y * 7), uint8(x * y), 255})
		}
	}
	colors := []MapColor{testQuantizeColor(255, 255, 255), testQuantizeColor(242, 127, 165), testQuantizeColor(51, 76, 178)}
	allowed := map[color.RGBA]bool{}
	for _, c := range colors {
		allowed[color.RGBA{uint8(c.R), uint8(c.G), uint8(c.B), 255}] = true
	}
	before := append([]MapColor(nil), colors...)
	for _, mode := range []string{"fs", "ordered", "none"} {
		for _, metric := range []string{"lab", "rgb"} {
			t.Run(mode+"/"+metric, func(t *testing.T) {
				opts := DefaultRenderOptions()
				opts.Dither, opts.ColorMetric, opts.Threshold = mode, metric, 0
				a := mustQuantize(t, src, nil, nil, opts, colors)
				b := mustQuantize(t, src, nil, nil, opts, colors)
				if a.Bounds() != src.Bounds() || !bytes.Equal(a.Pix, b.Pix) {
					t.Fatal("quantizer changed bounds or was nondeterministic")
				}
				for y := a.Bounds().Min.Y; y < a.Bounds().Max.Y; y++ {
					for x := a.Bounds().Min.X; x < a.Bounds().Max.X; x++ {
						if p := a.RGBAAt(x, y); !allowed[p] {
							t.Fatalf("color outside supplied palette: %v", p)
						}
					}
				}
			})
		}
	}
	for i := range colors {
		if colors[i] != before[i] {
			t.Fatal("quantization mutated palette")
		}
	}
}

func TestQuantizeZeroStrengthEqualsNone(t *testing.T) {
	src := quantizeRegressionSource().SubImage(image.Rect(24, 32, 64, 64)).(*image.RGBA)
	for _, metric := range []string{"lab", "rgb"} {
		opts := DefaultRenderOptions()
		opts.Dither, opts.ColorMetric, opts.Threshold = "none", metric, 0
		want := mustQuantize(t, src, nil, nil, opts, palette)
		for _, mode := range []string{"fs", "ordered"} {
			opts.Dither, opts.DitherStrength, opts.CleanHaze = mode, 0, true
			got := mustQuantize(t, src, nil, nil, opts, palette)
			if !bytes.Equal(want.Pix, got.Pix) {
				t.Fatalf("%s/%s at zero strength differs from none", mode, metric)
			}
		}
	}
}

func TestQuantizeMaskBlocksIncomingAndOutgoingError(t *testing.T) {
	const width, height = 32, 16
	a, b := image.NewRGBA(image.Rect(0, 0, width, height)), image.NewRGBA(image.Rect(0, 0, width, height))
	mask := make([]bool, width*height)
	smooth := make([]color.RGBA, width*height)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			a.SetRGBA(x, y, color.RGBA{250, 222, 199, 255})
			b.SetRGBA(x, y, color.RGBA{250, 222, 199, 255})
			if x >= 12 && x < 20 {
				mask[y*width+x] = true
				smooth[y*width+x] = color.RGBA{255, 255, 255, 255}
				a.SetRGBA(x, y, color.RGBA{0, 0, 0, 255})
				b.SetRGBA(x, y, color.RGBA{250, 20, 250, 255})
			}
		}
	}
	for _, mode := range []string{"fs", "ordered"} {
		for _, metric := range []string{"lab", "rgb"} {
			opts := DefaultRenderOptions()
			opts.Dither, opts.ColorMetric, opts.Threshold, opts.DitherStrength = mode, metric, 0, 1
			first := mustQuantize(t, a, mask, smooth, opts, palette)
			second := mustQuantize(t, b, mask, smooth, opts, palette)
			if !bytes.Equal(first.Pix, second.Pix) {
				t.Fatalf("%s/%s propagated masked source color", mode, metric)
			}
			for y := 0; y < height; y++ {
				for x := 12; x < 20; x++ {
					if first.RGBAAt(x, y) != (color.RGBA{255, 255, 255, 255}) {
						t.Fatalf("%s/%s received error inside mask", mode, metric)
					}
				}
			}
		}
	}
}

func TestQuantizeRGBAndOrderedRecoverPaleMixture(t *testing.T) {
	const width, height = 96, 96
	src := image.NewRGBA(image.Rect(0, 0, width, height))
	target := color.RGBA{252, 223, 233, 255}
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			src.SetRGBA(x, y, target)
		}
	}
	colors := []MapColor{testQuantizeColor(255, 255, 255), testQuantizeColor(242, 127, 165)}
	meanError := func(img *image.RGBA) float64 {
		var sums [3]float64
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				p := img.RGBAAt(x, y)
				sums[0], sums[1], sums[2] = sums[0]+float64(p.R), sums[1]+float64(p.G), sums[2]+float64(p.B)
			}
		}
		var errorSquared float64
		for ch, value := range []uint8{target.R, target.G, target.B} {
			d := sums[ch]/(width*height) - float64(value)
			errorSquared += d * d
		}
		return math.Sqrt(errorSquared)
	}
	opts := DefaultRenderOptions()
	opts.ColorMetric, opts.Dither, opts.Threshold, opts.DitherStrength = "rgb", "none", 0, 1
	nearestError := meanError(mustQuantize(t, src, nil, nil, opts, colors))
	for _, mode := range []string{"fs", "ordered"} {
		for _, metric := range []string{"rgb", "lab"} {
			if mode == "fs" && metric == "lab" {
				continue // Lab intentionally preserves the historical diffusion.
			}
			opts.Dither, opts.ColorMetric = mode, metric
			gotError := meanError(mustQuantize(t, src, nil, nil, opts, colors))
			t.Logf("%s/%s mean RGB error %.3f; nearest %.3f", mode, metric, gotError, nearestError)
			if gotError > 2 || gotError > nearestError*0.2 {
				t.Fatalf("%s/%s did not recover pale mixture: error %.3f", mode, metric, gotError)
			}
		}
	}
}

func TestOrderedRetainsMeanOfPaleYellowAndGreen(t *testing.T) {
	// Restricting mixtures to an anchored pair, or clipping the mixture's
	// residual, loses red/green in these pale colors from the portrait fixture.
	for _, target := range []color.RGBA{{250, 235, 176, 255}, {226, 238, 170, 255}, {232, 232, 232, 255}} {
		src := image.NewRGBA(image.Rect(0, 0, 64, 64))
		for y := 0; y < 64; y++ {
			for x := 0; x < 64; x++ {
				src.SetRGBA(x, y, target)
			}
		}
		opts := DefaultRenderOptions()
		opts.Dither, opts.ColorMetric, opts.DitherStrength, opts.Threshold = "ordered", "rgb", 1, 0
		out := mustQuantize(t, src, nil, nil, opts, palette)
		var sum [3]float64
		for i := 0; i < len(out.Pix); i += 4 {
			for ch := range sum {
				sum[ch] += float64(out.Pix[i+ch])
			}
		}
		for ch, value := range [3]uint8{target.R, target.G, target.B} {
			mean := sum[ch] / (64 * 64)
			if math.Abs(mean-float64(value)) > 8 {
				t.Fatalf("ordered lost the mean of %v: channel %d mean %.2f", target, ch, mean)
			}
		}
	}
}

func TestQuantizeGrayPaletteStaysGray(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 32, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			gray := uint8((x*7 + y*3) % 256)
			src.SetRGBA(x, y, color.RGBA{gray, gray, gray, 255})
		}
	}
	var grayColors []MapColor
	for _, c := range palette {
		if c.R == c.G && c.G == c.B {
			grayColors = append(grayColors, c)
		}
	}
	for _, mode := range []string{"fs", "ordered", "none"} {
		for _, metric := range []string{"lab", "rgb"} {
			opts := DefaultRenderOptions()
			opts.Monochrome, opts.Dither, opts.ColorMetric = true, mode, metric
			out := mustQuantize(t, src, nil, nil, opts, grayColors)
			for i := 0; i < len(out.Pix); i += 4 {
				if out.Pix[i] != out.Pix[i+1] || out.Pix[i+1] != out.Pix[i+2] {
					t.Fatalf("%s/%s produced a colored pixel in monochrome mode", mode, metric)
				}
			}
		}
	}
}

func TestQuantizeThresholdUsesDeltaEForBothMetrics(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 16, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			src.SetRGBA(x, y, color.RGBA{251, 249, 250, 255})
		}
	}
	for _, mode := range []string{"fs", "ordered"} {
		for _, metric := range []string{"lab", "rgb"} {
			opts := DefaultRenderOptions()
			opts.Dither, opts.ColorMetric, opts.DitherStrength = mode, metric, 1
			out := mustQuantize(t, src, nil, nil, opts, palette)
			for y := 0; y < 16; y++ {
				for x := 0; x < 16; x++ {
					if out.RGBAAt(x, y) != (color.RGBA{255, 255, 255, 255}) {
						t.Fatalf("%s/%s used RGB distance as threshold", mode, metric)
					}
				}
			}
		}
	}
}

func TestQuantizeRejectsInvalidInputs(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 2, 2))
	opts := DefaultRenderOptions()
	for _, change := range []func(*RenderOptions){
		func(o *RenderOptions) { o.Dither = "unknown" },
		func(o *RenderOptions) { o.ColorMetric = "unknown" },
		func(o *RenderOptions) { o.Threshold = math.NaN() },
		func(o *RenderOptions) { o.DitherStrength = math.Inf(1) },
		func(o *RenderOptions) { o.DitherStrength = -1 },
		func(o *RenderOptions) { o.DitherStrength = 2 },
	} {
		bad := opts
		change(&bad)
		if _, err := QuantizeImage(src, nil, nil, bad, palette); err == nil {
			t.Fatal("invalid option accepted")
		}
	}
	if _, err := QuantizeImage(src, nil, nil, opts, nil); err == nil {
		t.Fatal("empty palette accepted")
	}
	if _, err := QuantizeImage(nil, nil, nil, opts, palette); err == nil {
		t.Fatal("nil image accepted")
	}
	if _, err := QuantizeImage(src, []bool{true}, nil, opts, palette); err == nil {
		t.Fatal("short mask accepted")
	}
	if _, err := QuantizeImage(src, []bool{true, false, false, false}, nil, opts, palette); err == nil {
		t.Fatal("missing smoothed image accepted")
	}
}
