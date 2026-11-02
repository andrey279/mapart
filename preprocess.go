package main

import (
	"fmt"
	"image"
	"image/color"
	"math"

	"golang.org/x/image/draw"
)

// PrepareImage applies the requested source geometry and color adjustments.
// Defaults preserve the original white compositing and Catmull-Rom pipeline.
func PrepareImage(img image.Image, opts RenderOptions) (*image.RGBA, error) {
	if img == nil {
		return nil, fmt.Errorf("source image is nil")
	}
	if err := ValidateOptions(opts); err != nil {
		return nil, err
	}
	img = transformSource(img, opts)
	source, destination, err := preparationGeometry(img.Bounds(), opts)
	if err != nil {
		return nil, err
	}
	matte, err := ParseHexColor(opts.Matte)
	if err != nil {
		return nil, err
	}
	composited := image.NewRGBA(image.Rect(0, 0, source.Dx(), source.Dy()))
	draw.Draw(composited, composited.Bounds(), image.NewUniform(matte), image.Point{}, draw.Src)
	draw.Draw(composited, composited.Bounds(), img, source.Min, draw.Over)
	prepared := image.NewRGBA(image.Rect(0, 0, opts.MapsX*128, opts.MapsY*128))
	draw.Draw(prepared, prepared.Bounds(), image.NewUniform(matte), image.Point{}, draw.Src)
	var scaler draw.Interpolator = draw.CatmullRom
	switch opts.Resample {
	case "nearest":
		scaler = draw.NearestNeighbor
	case "bilinear":
		scaler = draw.BiLinear
	}
	scaler.Scale(prepared, destination, composited, composited.Bounds(), draw.Src, nil)
	adjustImageTones(prepared, destination, opts)
	if opts.Denoise > 0 {
		filterPreparedImage(prepared, destination, opts.Denoise, false)
	}
	if opts.Sharpen > 0 {
		filterPreparedImage(prepared, destination, opts.Sharpen, true)
	}
	return prepared, nil
}

// TransformMask accepts a mask matching either the source image or the prepared
// output. Source masks follow the same crop/fit geometry, with nearest sampling.
// When both sizes match, the mask is interpreted in source coordinates.
func TransformMask(mask image.Image, sourceBounds image.Rectangle, opts RenderOptions) ([]bool, error) {
	if mask == nil {
		return nil, fmt.Errorf("mask image is nil")
	}
	if err := ValidateOptions(opts); err != nil {
		return nil, err
	}
	if mask.Bounds().Size() == sourceBounds.Size() {
		mask = transformSource(mask, opts)
	}
	sourceBounds = transformedBounds(sourceBounds, opts.Rotation)
	source, destination, err := preparationGeometry(sourceBounds, opts)
	if err != nil {
		return nil, err
	}
	outputBounds := image.Rect(0, 0, opts.MapsX*128, opts.MapsY*128)
	bounds := mask.Bounds()
	var prepared image.Image
	if bounds.Size() == sourceBounds.Size() {
		binary := image.NewGray(bounds)
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				if maskSelected(mask.At(x, y)) {
					binary.SetGray(x, y, color.Gray{Y: 255})
				}
			}
		}
		scaled := image.NewGray(outputBounds)
		maskSource := source.Sub(sourceBounds.Min).Add(bounds.Min)
		draw.NearestNeighbor.Scale(scaled, destination, binary, maskSource, draw.Src, nil)
		prepared = scaled
	} else if bounds.Size() == outputBounds.Size() {
		prepared = mask
	} else {
		return nil, fmt.Errorf("mask size %dx%d must match source %dx%d or output %dx%d", bounds.Dx(), bounds.Dy(), sourceBounds.Dx(), sourceBounds.Dy(), outputBounds.Dx(), outputBounds.Dy())
	}
	selected := make([]bool, outputBounds.Dx()*outputBounds.Dy())
	for y := 0; y < outputBounds.Dy(); y++ {
		for x := 0; x < outputBounds.Dx(); x++ {
			if !image.Pt(x, y).In(destination) {
				continue
			}
			selected[y*outputBounds.Dx()+x] = maskSelected(prepared.At(prepared.Bounds().Min.X+x, prepared.Bounds().Min.Y+y))
		}
	}
	return selected, nil
}

func maskSelected(p color.Color) bool {
	r, g, b, a := p.RGBA()
	// RGBA returns premultiplied channels, so transparent white has no effect.
	return a != 0 && (299*r+587*g+114*b)/1000 > 32767
}

// Source coordinates include Bounds.Min; crop coordinates are relative to it.
func preparationGeometry(bounds image.Rectangle, opts RenderOptions) (image.Rectangle, image.Rectangle, error) {
	if bounds.Empty() {
		return image.Rectangle{}, image.Rectangle{}, fmt.Errorf("source image is empty")
	}
	source := bounds
	if opts.Crop != "" {
		crop, err := ParseCrop(opts.Crop)
		if err != nil {
			return image.Rectangle{}, image.Rectangle{}, err
		}
		localBounds := image.Rect(0, 0, bounds.Dx(), bounds.Dy())
		if crop.Empty() || !crop.In(localBounds) {
			return image.Rectangle{}, image.Rectangle{}, fmt.Errorf("crop %v lies outside source %dx%d", crop, bounds.Dx(), bounds.Dy())
		}
		source = crop.Add(bounds.Min)
	}
	destination := image.Rect(0, 0, opts.MapsX*128, opts.MapsY*128)
	sw, sh, dw, dh := source.Dx(), source.Dy(), destination.Dx(), destination.Dy()
	switch opts.Fit {
	case "contain":
		scale := math.Min(float64(dw)/float64(sw), float64(dh)/float64(sh))
		width := max(1, min(dw, int(math.Round(float64(sw)*scale))))
		height := max(1, min(dh, int(math.Round(float64(sh)*scale))))
		x, y := (dw-width)/2, (dh-height)/2
		destination = image.Rect(x, y, x+width, y+height)
	case "cover":
		if float64(sw)/float64(sh) > float64(dw)/float64(dh) {
			width := max(1, min(sw, int(math.Round(float64(sh)*float64(dw)/float64(dh)))))
			source.Min.X += (sw - width) / 2
			source.Max.X = source.Min.X + width
		} else {
			height := max(1, min(sh, int(math.Round(float64(sw)*float64(dh)/float64(dw)))))
			source.Min.Y += (sh - height) / 2
			source.Max.Y = source.Min.Y + height
		}
	}
	return source, destination, nil
}

func adjustImageTones(img *image.RGBA, area image.Rectangle, opts RenderOptions) {
	if !opts.AutoContrast && opts.Brightness == 0 && opts.Contrast == 1 && opts.Saturation == 1 && opts.Gamma == 1 && opts.Temperature == 0 {
		return
	}
	low, high := 0, 255
	if opts.AutoContrast {
		var histogram [256]int
		for y := area.Min.Y; y < area.Max.Y; y++ {
			for x := area.Min.X; x < area.Max.X; x++ {
				p := img.RGBAAt(x, y)
				luma := (299*int(p.R) + 587*int(p.G) + 114*int(p.B) + 500) / 1000
				histogram[luma]++
			}
		}
		count := area.Dx() * area.Dy()
		percentile := func(rank int) int {
			total := 0
			for level, n := range histogram {
				total += n
				if total >= rank {
					return level
				}
			}
			return 255
		}
		low = percentile(max(1, int(math.Ceil(float64(count)*0.01))))
		high = percentile(max(1, int(math.Ceil(float64(count)*0.99))))
		if high <= low {
			low, high = 0, 255
		}
	}
	for y := area.Min.Y; y < area.Max.Y; y++ {
		for x := area.Min.X; x < area.Max.X; x++ {
			p := img.RGBAAt(x, y)
			c := [3]float64{float64(p.R), float64(p.G), float64(p.B)}
			for i := range c {
				if low != 0 || high != 255 {
					c[i] = math.Max(0, math.Min(255, (c[i]-float64(low))*255/float64(high-low)))
				}
				c[i] += opts.Brightness * 255 / 100
				c[i] = (c[i]-127.5)*opts.Contrast + 127.5
			}
			luma := 0.2126*c[0] + 0.7152*c[1] + 0.0722*c[2]
			for i := range c {
				c[i] = luma + (c[i]-luma)*opts.Saturation
				if opts.Gamma != 1 {
					c[i] = math.Pow(math.Max(0, math.Min(255, c[i]))/255, 1/opts.Gamma) * 255
				}
			}
			c[0] += opts.Temperature * 0.5
			c[2] -= opts.Temperature * 0.5
			img.SetRGBA(x, y, color.RGBA{preparedByte(c[0]), preparedByte(c[1]), preparedByte(c[2]), 255})
		}
	}
}

func preparedByte(value float64) uint8 {
	return uint8(math.Round(math.Max(0, math.Min(255, value))))
}

// Work from a snapshot so results do not depend on scan direction. Neighbors
// clamp to the image content, keeping contain padding outside both filters.
func filterPreparedImage(img *image.RGBA, area image.Rectangle, strength float64, sharpen bool) {
	snapshot := &image.RGBA{Pix: append([]byte(nil), img.Pix...), Stride: img.Stride, Rect: img.Rect}
	for y := area.Min.Y; y < area.Max.Y; y++ {
		for x := area.Min.X; x < area.Max.X; x++ {
			var values [3][9]uint8
			index := 0
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					p := snapshot.RGBAAt(max(area.Min.X, min(area.Max.X-1, x+dx)), max(area.Min.Y, min(area.Max.Y-1, y+dy)))
					values[0][index], values[1][index], values[2][index] = p.R, p.G, p.B
					index++
				}
			}
			p := snapshot.RGBAAt(x, y)
			channels := [3]uint8{p.R, p.G, p.B}
			for channel, samples := range values {
				original := float64(channels[channel])
				if sharpen {
					total := 0
					for _, v := range samples {
						total += int(v)
					}
					channels[channel] = preparedByte(original + strength*(original-float64(total)/9))
				} else {
					for i := 1; i < len(samples); i++ {
						for j := i; j > 0 && samples[j] < samples[j-1]; j-- {
							samples[j], samples[j-1] = samples[j-1], samples[j]
						}
					}
					channels[channel] = preparedByte(original + strength*(float64(samples[4])-original))
				}
			}
			img.SetRGBA(x, y, color.RGBA{channels[0], channels[1], channels[2], 255})
		}
	}
}
