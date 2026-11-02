package main

import (
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	"image/png"
	"math"
	"os"
	"strings"
)

var baseColors = [16][3]float64{
	{255, 255, 255}, {216, 127, 51}, {178, 76, 216}, {102, 153, 216},
	{229, 229, 51}, {127, 204, 25}, {242, 127, 165}, {76, 76, 76},
	{153, 153, 153}, {76, 127, 153}, {127, 63, 178}, {51, 76, 178},
	{102, 76, 51}, {102, 127, 51}, {153, 51, 51}, {25, 25, 25},
}

type MapColor struct {
	R, G, B     float64
	L, A, B_lab float64
	Shade       string
	WoolID      int
}

var palette []MapColor

func rgbToLab(r, g, b float64) (L, A, B float64) {
	r, g, b = r/255.0, g/255.0, b/255.0
	linearize := func(v float64) float64 {
		if v <= 0.04045 {
			return v / 12.92
		}
		return math.Pow((v+0.055)/1.055, 2.4)
	}
	r, g, b = linearize(r), linearize(g), linearize(b)

	x := (r*0.4124 + g*0.3576 + b*0.1805) * 100.0
	y := (r*0.2126 + g*0.7152 + b*0.0722) * 100.0
	z := (r*0.0193 + g*0.1192 + b*0.9505) * 100.0

	x, y, z = x/95.047, y/100.000, z/108.883

	fx := math.Pow(x, 1.0/3.0)
	if x <= 0.008856 {
		fx = 7.787*x + 16.0/116.0
	}
	fy := math.Pow(y, 1.0/3.0)
	if y <= 0.008856 {
		fy = 7.787*y + 16.0/116.0
	}
	fz := math.Pow(z, 1.0/3.0)
	if z <= 0.008856 {
		fz = 7.787*z + 16.0/116.0
	}

	L = 116.0*fy - 16.0
	A = 500.0 * (fx - fy)
	B = 200.0 * (fy - fz)
	return
}

func init() {
	for id, c := range baseColors {
		rDark, gDark, bDark := c[0]*180/255, c[1]*180/255, c[2]*180/255
		lD, aD, bD := rgbToLab(rDark, gDark, bDark)
		palette = append(palette, MapColor{rDark, gDark, bDark, lD, aD, bD, "Dark", id})

		rNorm, gNorm, bNorm := c[0]*220/255, c[1]*220/255, c[2]*220/255
		lN, aN, bN := rgbToLab(rNorm, gNorm, bNorm)
		palette = append(palette, MapColor{rNorm, gNorm, bNorm, lN, aN, bN, "Normal", id})

		rLight, gLight, bLight := c[0]*255/255, c[1]*255/255, c[2]*255/255
		lL, aL, bL := rgbToLab(rLight, gLight, bLight)
		palette = append(palette, MapColor{rLight, gLight, bLight, lL, aL, bL, "Light", id})
	}
}

func ciede2000(L1, a1, b1, L2, a2, b2 float64) float64 {
	C1 := math.Sqrt(a1*a1 + b1*b1)
	C2 := math.Sqrt(a2*a2 + b2*b2)
	Cbar := (C1 + C2) / 2.0

	G := 0.5 * (1.0 - math.Sqrt(math.Pow(Cbar, 7.0)/(math.Pow(Cbar, 7.0)+math.Pow(25.0, 7.0))))
	a1_prime := (1.0 + G) * a1
	a2_prime := (1.0 + G) * a2

	C1_prime := math.Sqrt(a1_prime*a1_prime + b1*b1)
	C2_prime := math.Sqrt(a2_prime*a2_prime + b2*b2)
	Cbar_prime := (C1_prime + C2_prime) / 2.0

	h1_prime := math.Atan2(b1, a1_prime)
	if h1_prime < 0 {
		h1_prime += 2.0 * math.Pi
	}
	h2_prime := math.Atan2(b2, a2_prime)
	if h2_prime < 0 {
		h2_prime += 2.0 * math.Pi
	}

	Hbar_prime := h1_prime + h2_prime
	if math.Abs(h1_prime-h2_prime) > math.Pi {
		Hbar_prime += 2.0 * math.Pi
	}
	Hbar_prime /= 2.0

	T := 1.0 - 0.17*math.Cos(Hbar_prime-math.Pi/6.0) + 0.24*math.Cos(2.0*Hbar_prime) + 0.32*math.Cos(3.0*Hbar_prime+math.Pi/30.0) - 0.20*math.Cos(4.0*Hbar_prime-63.0*math.Pi/180.0)

	dh_prime := h2_prime - h1_prime
	if math.Abs(dh_prime) > math.Pi {
		if h2_prime <= h1_prime {
			dh_prime += 2.0 * math.Pi
		} else {
			dh_prime -= 2.0 * math.Pi
		}
	}

	dL_prime := L2 - L1
	dC_prime := C2_prime - C1_prime
	dH_prime := 2.0 * math.Sqrt(C1_prime*C2_prime) * math.Sin(dh_prime/2.0)

	Lbar := (L1 + L2) / 2.0
	Sl := 1.0 + (0.015*math.Pow(Lbar-50.0, 2.0))/math.Sqrt(20.0+math.Pow(Lbar-50.0, 2.0))
	Sc := 1.0 + 0.045*Cbar_prime
	Sh := 1.0 + 0.015*Cbar_prime*T

	dTheta := 30.0 * math.Pi / 180.0 * math.Exp(-math.Pow((Hbar_prime-275.0*math.Pi/180.0)/(25.0*math.Pi/180.0), 2.0))
	Rc := 2.0 * math.Sqrt(math.Pow(Cbar_prime, 7.0)/(math.Pow(Cbar_prime, 7.0)+math.Pow(25.0, 7.0)))
	Rt := -math.Sin(2.0*dTheta) * Rc

	return math.Sqrt(math.Pow(dL_prime/Sl, 2.0) + math.Pow(dC_prime/Sc, 2.0) + math.Pow(dH_prime/Sh, 2.0) + Rt*(dC_prime/Sc)*(dH_prime/Sh))
}

func printProgressBar(iteration, total int, prefix, suffix string, length int) {
	percent := float64(iteration) / float64(total)
	filledLength := int(float64(length) * percent)
	bar := strings.Repeat("#", filledLength) + strings.Repeat("-", length-filledLength)
	fmt.Printf("\r%s [%s] %.1f%% %s", prefix, bar, percent*100, suffix)
	if iteration == total {
		fmt.Println()
	}
}

func savePNG(path string, img image.Image) (err error) {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := f.Close(); err == nil {
			err = closeErr
		}
	}()
	return png.Encode(f, img)
}

// detectBackground finds smooth regions connected to the image border whose
// colors match the dominant border color. Smoothing also suppresses source
// noise before palette matching in those regions.
func detectBackground(img *image.RGBA) ([]bool, []color.RGBA) {
	return detectBackgroundWithOptions(img, DefaultRenderOptions())
}

func detectBackgroundWithOptions(img *image.RGBA, opts RenderOptions) ([]bool, []color.RGBA) {
	width, height := img.Bounds().Dx(), img.Bounds().Dy()
	smooth := make([]color.RGBA, width*height)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			var r, g, b, count int
			for dy := -1; dy <= 1; dy++ {
				ny := y + dy
				if ny < 0 || ny >= height {
					continue
				}
				for dx := -1; dx <= 1; dx++ {
					nx := x + dx
					if nx < 0 || nx >= width {
						continue
					}
					p := img.RGBAAt(img.Rect.Min.X+nx, img.Rect.Min.Y+ny)
					r, g, b, count = r+int(p.R), g+int(p.G), b+int(p.B), count+1
				}
			}
			smooth[y*width+x] = color.RGBA{uint8(r / count), uint8(g / count), uint8(b / count), 255}
		}
	}

	// Quantize border samples to find the largest color group. This avoids
	// picking a foreground object that happens to touch one edge.
	type bucket struct{ count, r, g, b int }
	var buckets [512]bucket
	visitBorder := func(fn func(int)) {
		for x := 0; x < width; x++ {
			fn(x)
			fn((height-1)*width + x)
		}
		for y := 1; y < height-1; y++ {
			fn(y * width)
			fn(y*width + width - 1)
		}
	}
	visitBorder(func(i int) {
		p := smooth[i]
		key := int(p.R/32)<<6 | int(p.G/32)<<3 | int(p.B/32)
		v := &buckets[key]
		v.count++
		v.r += int(p.R)
		v.g += int(p.G)
		v.b += int(p.B)
	})
	best := bucket{}
	for _, v := range buckets {
		if v.count > best.count {
			best = v
		}
	}
	background := make([]bool, width*height)
	// Pale sky can occupy only a small part of the border. Include it even
	// when the plain-background checks below reject the overall scene.
	if opts.SkyCleanup {
		defer suppressSkyDithering(img, background, smooth)
	}
	if opts.Background == "off" {
		return background, smooth
	}
	if best.count == 0 {
		return background, smooth
	}
	base := color.RGBA{uint8(best.r / best.count), uint8(best.g / best.count), uint8(best.b / best.count), 255}
	if opts.Background == "color" {
		base, _ = ParseHexColor(opts.BackgroundColor) // validated before rendering
	}
	distanceSquared := func(a, b color.RGBA) int {
		dr, dg, db := int(a.R)-int(b.R), int(a.G)-int(b.G), int(a.B)-int(b.B)
		return dr*dr + dg*dg + db*db
	}
	// Selective dithering is useful for a plain backdrop. On a detailed
	// scene, suppressing dithering would create irregular flat patches.
	borderCount, matchingBorder := 0, 0
	visitBorder(func(i int) {
		borderCount++
		if float64(distanceSquared(smooth[i], base)) <= opts.BackgroundTolerance*opts.BackgroundTolerance {
			matchingBorder++
		}
	})
	if opts.Background == "auto" && matchingBorder*10 < borderCount*7 {
		return background, smooth
	}
	// Keep the same color tolerance as the border check. A much wider flood
	// tolerance swallowed gradual changes in blue skies, water and pale art,
	// turning distinct tones into a supposedly flat backdrop.
	maxBaseDistance := opts.BackgroundTolerance * opts.BackgroundTolerance
	const maxNeighborDistance = 24 * 24
	queue := make([]int, 0, width+height)
	visitBorder(func(i int) {
		if !background[i] && float64(distanceSquared(smooth[i], base)) <= maxBaseDistance {
			background[i] = true
			queue = append(queue, i)
		}
	})
	for head := 0; head < len(queue); head++ {
		i := queue[head]
		x, y := i%width, i/width
		for _, n := range [][2]int{{x - 1, y}, {x + 1, y}, {x, y - 1}, {x, y + 1}} {
			if n[0] < 0 || n[0] >= width || n[1] < 0 || n[1] >= height {
				continue
			}
			j := n[1]*width + n[0]
			if background[j] || float64(distanceSquared(smooth[j], base)) > maxBaseDistance || distanceSquared(smooth[j], smooth[i]) > maxNeighborDistance {
				continue
			}
			background[j] = true
			queue = append(queue, j)
		}
	}
	if opts.Background == "auto" && len(queue) < width*height/30 {
		clear(background)
		return background, smooth
	}

	// A gap between a figure's body and arm can be enclosed by foreground.
	// Include sizable, smooth regions that closely match the detected sky.
	maxEnclosedDistance := min(28*28, maxBaseDistance)
	seen := make([]bool, len(background))
	for start := range background {
		if background[start] || seen[start] || float64(distanceSquared(smooth[start], base)) > maxEnclosedDistance {
			continue
		}
		component := []int{start}
		seen[start] = true
		for head := 0; head < len(component); head++ {
			i := component[head]
			x, y := i%width, i/width
			for _, n := range [][2]int{{x - 1, y}, {x + 1, y}, {x, y - 1}, {x, y + 1}} {
				if n[0] < 0 || n[0] >= width || n[1] < 0 || n[1] >= height {
					continue
				}
				j := n[1]*width + n[0]
				if background[j] || seen[j] || float64(distanceSquared(smooth[j], base)) > maxEnclosedDistance || distanceSquared(smooth[j], smooth[i]) > maxNeighborDistance {
					continue
				}
				seen[j] = true
				component = append(component, j)
			}
		}
		if len(component) >= 32 {
			for _, i := range component {
				background[i] = true
			}
		}
	}
	return background, smooth
}

func processImage(img image.Image, outPath string, mapsX, mapsY int, ditherThresh float64) error {
	return processImageWithCleanup(img, outPath, mapsX, mapsY, ditherThresh, false)
}

func processImageWithCleanup(img image.Image, outPath string, mapsX, mapsY int, ditherThresh float64, cleanHaze bool) error {
	opts := DefaultRenderOptions()
	opts.MapsX, opts.MapsY, opts.Threshold, opts.CleanHaze = mapsX, mapsY, ditherThresh, cleanHaze
	return processConfiguredImage(img, outPath, opts, false)
}

func main() {
	var err error
	if len(os.Args) == 1 {
		err = runGUI(DefaultRenderOptions(), "", DefaultBuildOptions())
	} else {
		err = runCLI(os.Args[1:])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		reportGUIStartupError(err)
		os.Exit(1)
	}
}
