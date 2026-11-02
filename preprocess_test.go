package main

import (
	"bytes"
	"image"
	"image/color"
	"testing"

	"golang.org/x/image/draw"
)

func TestPrepareDefaultsMatchPreviousAlphaAndResize(t *testing.T) {
	src := image.NewNRGBA(image.Rect(9, -4, 16, 7))
	for y := src.Rect.Min.Y; y < src.Rect.Max.Y; y++ {
		for x := src.Rect.Min.X; x < src.Rect.Max.X; x++ {
			src.SetNRGBA(x, y, color.NRGBA{uint8(x * 17), uint8((y + 4) * 23), 147, []uint8{0, 63, 128, 255}[(x+y+4)%4]})
		}
	}
	opts := DefaultRenderOptions()
	opts.MapsX, opts.MapsY = 1, 1
	got, err := PrepareImage(src, opts)
	if err != nil {
		t.Fatal(err)
	}
	// Reference the previous pipeline, including its alpha-compositing order.
	composited := image.NewRGBA(image.Rect(0, 0, src.Bounds().Dx(), src.Bounds().Dy()))
	draw.Draw(composited, composited.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.Draw(composited, composited.Bounds(), src, src.Bounds().Min, draw.Over)
	want := image.NewRGBA(image.Rect(0, 0, 128, 128))
	draw.CatmullRom.Scale(want, want.Bounds(), composited, composited.Bounds(), draw.Src, nil)
	if got.Bounds() != want.Bounds() || !bytes.Equal(got.Pix, want.Pix) {
		t.Fatal("default preprocessing differs from the previous pixels")
	}
}

func TestPrepareFitAndMaskStayAlignedAfterCrop(t *testing.T) {
	src := image.NewRGBA(image.Rect(10, 20, 18, 24))
	mask := image.NewGray(image.Rect(-5, 6, 3, 10))
	for y := 0; y < 4; y++ {
		for x := 0; x < 8; x++ {
			level := uint8(0)
			if x < 4 {
				level = 255
			}
			src.SetRGBA(10+x, 20+y, color.RGBA{level, level, level, 255})
			mask.SetGray(-5+x, 6+y, color.Gray{Y: level})
		}
	}
	for _, fit := range []string{"stretch", "contain", "cover"} {
		t.Run(fit, func(t *testing.T) {
			opts := DefaultRenderOptions()
			opts.MapsX, opts.MapsY = 1, 1
			opts.Fit, opts.Resample, opts.Crop, opts.Matte = fit, "nearest", "1,0,6,4", "#ff00ff"
			got, err := PrepareImage(src, opts)
			if err != nil {
				t.Fatal(err)
			}
			selected, err := TransformMask(mask, src.Bounds(), opts)
			if err != nil {
				t.Fatal(err)
			}
			if len(selected) != 128*128 {
				t.Fatalf("unexpected mask length %d", len(selected))
			}
			for y := 0; y < 128; y++ {
				for x := 0; x < 128; x++ {
					white := got.RGBAAt(x, y) == (color.RGBA{255, 255, 255, 255})
					if selected[y*128+x] != white {
						t.Fatalf("image and mask disagree at (%d,%d)", x, y)
					}
				}
			}
			if fit == "contain" {
				if got.RGBAAt(0, 20) != (color.RGBA{255, 0, 255, 255}) || !selected[21*128] || selected[106*128] {
					t.Fatal("contain content or padding is not centered")
				}
			} else if !selected[0] || selected[127] {
				t.Fatal("stretch/cover lost the centered white-left, black-right content")
			}
		})
	}
}

func TestTransformReadyMaskThresholdAndAlpha(t *testing.T) {
	opts := DefaultRenderOptions()
	opts.MapsX, opts.MapsY = 1, 1
	mask := image.NewNRGBA(image.Rect(7, 11, 135, 139))
	mask.SetNRGBA(7, 11, color.NRGBA{255, 255, 255, 0})
	mask.SetNRGBA(8, 11, color.NRGBA{127, 127, 127, 255})
	mask.SetNRGBA(9, 11, color.NRGBA{128, 128, 128, 255})
	got, err := TransformMask(mask, image.Rect(0, 0, 8, 4), opts)
	if err != nil {
		t.Fatal(err)
	}
	if got[0] || got[1] || !got[2] {
		t.Fatal("transparent white or the grayscale threshold was handled incorrectly")
	}
	if _, err := TransformMask(image.NewGray(image.Rect(0, 0, 7, 4)), image.Rect(0, 0, 8, 4), opts); err == nil {
		t.Fatal("accepted a mask with unrelated dimensions")
	}
	opts.Fit = "contain"
	draw.Draw(mask, mask.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	got, err = TransformMask(mask, image.Rect(0, 0, 8, 4), opts)
	if err != nil {
		t.Fatal(err)
	}
	if got[0] || !got[64*128+64] || got[len(got)-1] {
		t.Fatal("a ready output mask selected contain padding")
	}
}

func TestPrepareRejectsCropOutsideSource(t *testing.T) {
	src := image.NewRGBA(image.Rect(10, 20, 18, 24))
	for _, crop := range []string{"7,0,2,4", "0,3,8,2", "-1,0,2,2", "0,0,0,2"} {
		opts := DefaultRenderOptions()
		opts.Crop = crop
		if _, err := PrepareImage(src, opts); err == nil {
			t.Errorf("accepted invalid crop %q", crop)
		}
		if _, err := TransformMask(src, src.Bounds(), opts); err == nil {
			t.Errorf("mask accepted invalid crop %q", crop)
		}
	}
}

func TestPrepareFiltersKeepConstantAndRemoveImpulse(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 128, 128))
	draw.Draw(src, src.Bounds(), image.NewUniform(color.RGBA{32, 91, 173, 255}), image.Point{}, draw.Src)
	opts := DefaultRenderOptions()
	opts.MapsX, opts.MapsY, opts.Sharpen, opts.Denoise = 1, 1, 2, 1
	got, err := PrepareImage(src, opts)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Pix, src.Pix) {
		t.Fatal("denoise or sharpening altered a constant color, including at edges")
	}
	src.SetRGBA(64, 64, color.RGBA{255, 255, 255, 255})
	opts.Sharpen = 0
	got, err = PrepareImage(src, opts)
	if err != nil {
		t.Fatal(err)
	}
	if got.RGBAAt(64, 64) != (color.RGBA{32, 91, 173, 255}) {
		t.Fatal("median denoise did not remove an isolated impulse")
	}
}

func TestPrepareGammaAndSaturationPreserveMatte(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 8, 4))
	draw.Draw(src, src.Bounds(), image.NewUniform(color.RGBA{64, 64, 64, 255}), image.Point{}, draw.Src)
	opts := DefaultRenderOptions()
	opts.MapsX, opts.MapsY, opts.Gamma = 1, 1, 2
	opts.Fit, opts.Matte = "contain", "#102030"
	got, err := PrepareImage(src, opts)
	if err != nil {
		t.Fatal(err)
	}
	if got.RGBAAt(64, 64) != (color.RGBA{128, 128, 128, 255}) || got.RGBAAt(0, 0) != (color.RGBA{16, 32, 48, 255}) {
		t.Fatal("gamma did not lighten the image or altered matte padding")
	}
	draw.Draw(src, src.Bounds(), image.NewUniform(color.RGBA{128, 80, 40, 255}), image.Point{}, draw.Src)
	opts.Gamma, opts.Saturation = 1, 0
	got, err = PrepareImage(src, opts)
	if err != nil {
		t.Fatal(err)
	}
	if got.RGBAAt(64, 64) != (color.RGBA{87, 87, 87, 255}) {
		t.Fatal("zero saturation did not preserve luminance")
	}
}
