package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"strings"

	"github.com/makiuchi-d/gozxing"
	qrreader "github.com/makiuchi-d/gozxing/qrcode"
	qrencoder "github.com/skip2/go-qrcode"
	xdraw "golang.org/x/image/draw"
)

const generatedQRSource = "QR-код (из генератора)"

type QRCodeOptions struct {
	Text                                          string
	MapsX, MapsY                                  int
	BuildMode, Style                              string
	ForegroundBlock, BackgroundBlock, FinderBlock string
	LogoPath                                      string
	LogoPercent                                   int
}

func DefaultQRCodeOptions() QRCodeOptions {
	return QRCodeOptions{MapsX: 1, MapsY: 1, BuildMode: "2d", Style: "square",
		ForegroundBlock: "black_wool", BackgroundBlock: "white_wool", FinderBlock: "black_wool", LogoPercent: 15}
}

var qrStyleLabels = []string{"Квадраты", "Скруглённые", "Круги"}
var qrStyles = []string{"square", "rounded", "dots"}
var qrModeLabels = []string{"2D — плоская", "3D — лестница"}

func qrBlockLabels() []string {
	labels := make([]string, paletteBlockCount)
	for id := range labels {
		labels[id] = blockLabel(id)
	}
	return labels
}

func qrBlockIndex(name string) int {
	ids, err := selectedBlockIDs(name)
	if err == nil && len(ids) == 1 {
		for id := range ids {
			return id
		}
	}
	return -1
}

func qrBlockName(id int) string {
	return strings.TrimPrefix(blockSelectionName(id), "minecraft:")
}

func qrBlockColour(name, shade string) (color.RGBA, error) {
	id := qrBlockIndex(name)
	if id < 0 {
		return color.RGBA{}, fmt.Errorf("для QR выберите один материал из палитры блоков: %q", name)
	}
	for _, p := range append(append(append(append([]MapColor(nil), palette...), terracottaPalette()...), naturalPalette()...), extraPalette()...) {
		if p.WoolID == id && p.Shade == shade {
			return palettePixel(p), nil
		}
	}
	return color.RGBA{}, fmt.Errorf("нет цвета блока %q", name)
}

type QRCodeResult struct {
	Source       *image.RGBA
	Preview      *PreviewResult
	Options      RenderOptions
	Modules      int // Symbol width without the four-module quiet zone.
	ModulePixels int
}

// QRRenderOptions deliberately uses crisp, unfiltered block colours. Styling
// takes place on an integer module grid before the normal map-art pipeline.
func QRRenderOptions(q QRCodeOptions) RenderOptions {
	opts := DefaultRenderOptions()
	opts.MapsX, opts.MapsY, opts.BuildMode = q.MapsX, q.MapsY, q.BuildMode
	opts.Dither, opts.Background, opts.SkyCleanup = "none", "off", false
	opts.Fit, opts.Resample = "contain", "nearest"
	ids := map[int]bool{}
	for _, name := range []string{q.ForegroundBlock, q.BackgroundBlock, q.FinderBlock} {
		if id := qrBlockIndex(name); id >= 0 {
			ids[id] = true
		}
	}
	if strings.TrimSpace(q.LogoPath) != "" {
		// The logo can use all available Minecraft colours.
		for id := 0; id < paletteBlockCount; id++ {
			ids[id] = true
		}
	}
	opts.SelectedBlocks = blockSelection(ids)
	bg, err := qrBlockColour(q.BackgroundBlock, "Normal")
	if err == nil {
		opts.Matte = fmt.Sprintf("#%02X%02X%02X", bg.R, bg.G, bg.B)
	}
	return opts
}

func GenerateQRCode(q QRCodeOptions) (*QRCodeResult, error) {
	if strings.TrimSpace(q.Text) == "" {
		return nil, fmt.Errorf("вставьте ссылку или текст для QR-кода")
	}
	if q.BuildMode != "2d" && q.BuildMode != "3d" {
		return nil, fmt.Errorf("режим QR-постройки: 2d или 3d")
	}
	if q.Style != "square" && q.Style != "rounded" && q.Style != "dots" {
		return nil, fmt.Errorf("форма элементов QR: square, rounded или dots")
	}
	if q.LogoPercent < 5 || q.LogoPercent > 20 {
		return nil, fmt.Errorf("размер логотипа: от 5 до 20%% ширины QR-кода")
	}
	opts := QRRenderOptions(q)
	if err := ValidateOptions(opts); err != nil {
		return nil, err
	}
	fgShade, bgShade := "Normal", "Normal"
	if q.BuildMode == "3d" {
		fgShade, bgShade = "Dark", "Light"
	}
	fg, err := qrBlockColour(q.ForegroundBlock, fgShade)
	if err != nil {
		return nil, err
	}
	bg, err := qrBlockColour(q.BackgroundBlock, bgShade)
	if err != nil {
		return nil, err
	}
	finder, err := qrBlockColour(q.FinderBlock, fgShade)
	if err != nil {
		return nil, err
	}
	if qrLuminance(bg)-max(qrLuminance(fg), qrLuminance(finder)) < 50 {
		return nil, fmt.Errorf("выберите светлый фон и более тёмные блоки для элементов и углов QR-кода")
	}
	code, err := qrencoder.New(q.Text, qrencoder.Highest)
	if err != nil {
		return nil, fmt.Errorf("не удалось создать QR-код: %w", err)
	}
	code.DisableBorder = true
	bits := code.Bitmap()
	n := len(bits)
	w, h := q.MapsX*128, q.MapsY*128
	module := min(w, h) / (n + 8)
	minimum := 2
	if q.Style != "square" {
		minimum = 3
	}
	if module < minimum {
		return nil, fmt.Errorf("QR-код слишком плотный для %d × %d карт: увеличьте размер или сократите ссылку (нужно минимум %d блока на элемент)", q.MapsX, q.MapsY, minimum)
	}
	source := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(source, source.Bounds(), image.NewUniform(bg), image.Point{}, draw.Src)
	x0, y0 := (w-n*module)/2, (h-n*module)/2
	for y, row := range bits {
		for x, on := range row {
			if !on {
				continue
			}
			c, style := fg, q.Style
			if qrFinderModule(x, y, n) {
				c, style = finder, "square"
			} else if x == 6 || y == 6 {
				// Keep the timing tracks intact in every style.
				style = "square"
			}
			qrDrawModule(source, image.Rect(x0+x*module, y0+y*module, x0+(x+1)*module, y0+(y+1)*module), c, style)
		}
	}
	if strings.TrimSpace(q.LogoPath) != "" {
		logo, err := decodeLocalImage(strings.TrimSpace(q.LogoPath))
		if err != nil {
			return nil, fmt.Errorf("логотип: %w", err)
		}
		// Align the central overlay to whole modules; its white padding is part
		// of the size limit. Error correction is always at the highest level.
		logoModules := max(1, n*q.LogoPercent/100)
		if logoModules%2 == 0 {
			logoModules--
		}
		start := (n - logoModules) / 2
		r := image.Rect(x0+start*module, y0+start*module, x0+(start+logoModules)*module, y0+(start+logoModules)*module)
		draw.Draw(source, r, image.NewUniform(bg), image.Point{}, draw.Src)
		inset := max(1, module/2)
		target := r.Inset(inset)
		if !target.Empty() {
			scale := min(float64(target.Dx())/float64(logo.Bounds().Dx()), float64(target.Dy())/float64(logo.Bounds().Dy()))
			lw, lh := max(1, int(math.Round(float64(logo.Bounds().Dx())*scale))), max(1, int(math.Round(float64(logo.Bounds().Dy())*scale)))
			target = image.Rect(target.Min.X+(target.Dx()-lw)/2, target.Min.Y+(target.Dy()-lh)/2, target.Min.X+(target.Dx()-lw)/2+lw, target.Min.Y+(target.Dy()-lh)/2+lh)
			xdraw.CatmullRom.Scale(source, target, logo, logo.Bounds(), draw.Over, nil)
		}
	}
	preview, err := RenderPreview(source, opts)
	if err != nil {
		return nil, err
	}
	mapImage, err := qrExactMapImage(preview)
	if err != nil {
		return nil, err
	}
	if err := VerifyQRCode(mapImage, q.Text); err != nil {
		return nil, fmt.Errorf("QR-код не читается после перевода в блоки: увеличьте размер карт, выберите контрастные цвета или уменьшите логотип")
	}
	return &QRCodeResult{Source: source, Preview: preview, Options: opts, Modules: n, ModulePixels: module}, nil
}

func qrLuminance(c color.RGBA) float64 {
	return .299*float64(c.R) + .587*float64(c.G) + .114*float64(c.B)
}

func qrFinderModule(x, y, n int) bool {
	return (x < 8 && y < 8) || (x >= n-8 && y < 8) || (x < 8 && y >= n-8)
}

func qrDrawModule(img *image.RGBA, r image.Rectangle, c color.RGBA, style string) {
	if style == "square" {
		draw.Draw(img, r, image.NewUniform(c), image.Point{}, draw.Src)
		return
	}
	size := float64(r.Dx())
	radius := max(1.0, size/2-.25)
	for y := 0; y < r.Dy(); y++ {
		for x := 0; x < r.Dx(); x++ {
			dx, dy := math.Abs(float64(x)+.5-size/2), math.Abs(float64(y)+.5-size/2)
			on := dx*dx+dy*dy <= radius*radius
			if style == "rounded" {
				radius := max(1.0, size/3)
				dx, dy = max(0, dx-(size/2-radius)), max(0, dy-(size/2-radius))
				on = dx*dx+dy*dy <= radius*radius
			}
			if on {
				img.SetRGBA(r.Min.X+x, r.Min.Y+y, c)
			}
		}
	}
}

// Match Java's integer map palette, including the one-channel rounding
// differences between the quantizer and an exported Minecraft map.
func qrExactMapImage(result *PreviewResult) (*image.RGBA, error) {
	lookup, err := structureColorLookup(result)
	if err != nil {
		return nil, err
	}
	img := image.NewRGBA(result.Image.Bounds())
	for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
		for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
			p, ok := lookup[result.Image.RGBAAt(x, y)]
			if !ok {
				return nil, fmt.Errorf("пиксель QR-кода отсутствует в палитре блоков")
			}
			_, base, _ := paletteBlock(p.WoolID)
			mod := map[string]int{"Dark": 180, "Normal": 220, "Light": 255}[p.Shade]
			img.SetRGBA(x, y, color.RGBA{uint8(int(base[0]) * mod / 255), uint8(int(base[1]) * mod / 255), uint8(int(base[2]) * mod / 255), 255})
		}
	}
	return img, nil
}

// VerifyQRCode uses a decoder rather than inspecting the generated matrix.
// Small map images are enlarged with nearest-neighbour sampling for detection.
func VerifyQRCode(img image.Image, text string) error {
	if img == nil || img.Bounds().Empty() {
		return fmt.Errorf("нет изображения QR-кода")
	}
	// Remove uniform outer margins before enlarging. A rectangular map art
	// may be very wide while its QR symbol still occupies a small square.
	bounds := img.Bounds()
	background := color.RGBAModel.Convert(img.At(bounds.Min.X, bounds.Min.Y))
	occupied := image.Rectangle{}
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			if color.RGBAModel.Convert(img.At(x, y)) != background {
				occupied = occupied.Union(image.Rect(x, y, x+1, y+1))
			}
		}
	}
	if occupied.Empty() {
		return fmt.Errorf("QR-код не читается: изображение одноцветное")
	}
	margin := max(8, max(occupied.Dx(), occupied.Dy())/5)
	crop := occupied.Inset(-margin).Intersect(bounds)
	if crop != bounds {
		trimmed := image.NewRGBA(image.Rect(0, 0, crop.Dx(), crop.Dy()))
		draw.Draw(trimmed, trimmed.Bounds(), img, crop.Min, draw.Src)
		img = trimmed
	}
	if size := min(img.Bounds().Dx(), img.Bounds().Dy()); size < 512 {
		scale := (512 + size - 1) / size
		large := image.NewRGBA(image.Rect(0, 0, img.Bounds().Dx()*scale, img.Bounds().Dy()*scale))
		xdraw.NearestNeighbor.Scale(large, large.Bounds(), img, img.Bounds(), draw.Src, nil)
		img = large
	}
	bitmap, err := gozxing.NewBinaryBitmapFromImage(img)
	if err != nil {
		return err
	}
	decoded, err := qrreader.NewQRCodeReader().Decode(bitmap, map[gozxing.DecodeHintType]interface{}{gozxing.DecodeHintType_TRY_HARDER: true, gozxing.DecodeHintType_CHARACTER_SET: "UTF-8"})
	if err != nil {
		return fmt.Errorf("QR-код не читается: %w", err)
	}
	if decoded.GetText() != text {
		return fmt.Errorf("QR-код содержит другой текст")
	}
	return nil
}

// Generated sources remain editable and can be reprocessed just like files.
func studioSourceImage(path string, generated image.Image) (image.Image, error) {
	if path == generatedQRSource && generated != nil {
		return generated, nil
	}
	return decodeLocalImage(path)
}

func qrReadabilityStatus(result *PreviewResult, structure *Structure, text string) string {
	var img image.Image = result.Image
	if structure != nil {
		img = structure.MapImage
	}
	if err := VerifyQRCode(img, text); err != nil {
		return "QR-код сейчас не читается. Откройте генератор или отмените правки, которые изменили код."
	}
	return "QR-код проверен: ссылка или текст читается. Можно редактировать карту и сохранить PNG, NBT или ZIP."
}
