//go:build !windows && gui

package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
)

// Downsampling a dithered map with nearest-neighbour sampling drops whole
// blocks and creates moire and false colours. Smooth only the reduced preview;
// keep original pixels when zoomed in and leave the export image untouched.
type studioPreviewLayout struct {
	image  *canvas.Image
	pixels bool
}

func (l studioPreviewLayout) MinSize([]fyne.CanvasObject) fyne.Size { return l.image.MinSize() }

func (l studioPreviewLayout) Layout(_ []fyne.CanvasObject, size fyne.Size) {
	img := l.image
	img.Move(fyne.Position{})
	img.Resize(size)
	mode := canvas.ImageScaleSmooth
	if l.pixels {
		mode = canvas.ImageScalePixels
		if img.Image != nil && img.FillMode != canvas.ImageFillOriginal {
			density := float32(1)
			if c := fyne.CurrentApp().Driver().CanvasForObject(img); c != nil {
				density = c.Scale()
			}
			bounds := img.Image.Bounds()
			if size.Width*density < float32(bounds.Dx()) || size.Height*density < float32(bounds.Dy()) {
				mode = canvas.ImageScaleSmooth
			}
		}
	}
	if img.ScaleMode != mode {
		img.ScaleMode = mode
		img.Refresh()
	}
}
