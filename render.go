package main

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	"math"
	"os"
	"path/filepath"
	"strings"

	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
)

type PreviewResult struct {
	Original, Prepared, Image *image.RGBA
	NoDitherMask              []bool
	Palette                   []MapColor
	BuildMode                 string
}

func decodeLocalImage(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("декодирование %s: %w", path, err)
	}
	return img, nil
}

func paletteForOptions(opts RenderOptions) ([]MapColor, error) {
	colors := append([]MapColor(nil), palette...)
	ids := activeBlockIDs(opts)
	exact := opts.AllBlocks || usesRemovedBlocks(opts.SelectedBlocks)
	for id, selected := range ids {
		exact = exact || (selected && id >= legacyBlockCount)
	}
	if exact {
		// Full-catalogue aliases share Java's integer map colours. Preserve
		// historical rounding only for existing wool/terracotta-only profiles.
		for i, p := range colors {
			_, base, _ := paletteBlock(p.WoolID)
			modifier := []int{180, 220, 255}[i%3]
			r, g, b := float64(int(base[0])*modifier/255), float64(int(base[1])*modifier/255), float64(int(base[2])*modifier/255)
			l, a, lb := rgbToLab(r, g, b)
			colors[i] = MapColor{r, g, b, l, a, lb, p.Shade, p.WoolID}
		}
	}
	if opts.AllBlocks || opts.Terracotta || opts.SelectedBlocks != "" {
		colors = append(colors, terracottaPalette()...)
	}
	if opts.AllBlocks || opts.NaturalBlocks || opts.SelectedBlocks != "" {
		colors = append(colors, naturalPalette()...)
	}
	if opts.AllBlocks || opts.SelectedBlocks != "" {
		colors = append(colors, extraPalette()...)
	}
	if opts.SelectedBlocks != "" || opts.BuildMode == "2d" {
		filtered := make([]MapColor, 0, len(colors))
		for _, p := range colors {
			if ids[p.WoolID] && (opts.BuildMode != "2d" || p.Shade == "Normal") {
				filtered = append(filtered, p)
			}
		}
		colors = filtered
	}
	if opts.PaletteFile != "" {
		data, err := os.ReadFile(opts.PaletteFile)
		if err != nil {
			return nil, fmt.Errorf("палитра: %w", err)
		}
		var entries []string
		if err := json.Unmarshal(data, &entries); err != nil {
			return nil, fmt.Errorf("палитра должна быть JSON-массивом #RRGGBB: %w", err)
		}
		if len(entries) < 2 || len(entries) > 256 {
			return nil, fmt.Errorf("палитра должна содержать от 2 до 256 цветов")
		}
		colors = nil
		seen := map[color.RGBA]bool{}
		for _, value := range entries {
			p, err := ParseHexColor(value)
			if err != nil {
				return nil, fmt.Errorf("палитра: %w", err)
			}
			if seen[p] {
				continue
			}
			seen[p] = true
			r, g, b := float64(p.R), float64(p.G), float64(p.B)
			l, a, lb := rgbToLab(r, g, b)
			colors = append(colors, MapColor{r, g, b, l, a, lb, "Custom", len(colors)})
		}
	}
	if opts.Monochrome {
		neutral := colors[:0]
		for _, p := range colors {
			if math.Abs(p.R-p.G) < .01 && math.Abs(p.G-p.B) < .01 {
				neutral = append(neutral, p)
			}
		}
		colors = neutral
	}
	if opts.AllBlocks || opts.SelectedBlocks != "" {
		// Material aliases do not add colours. Keep the first enabled material,
		// matching quantization and NBT export, without comparing 2000 aliases
		// for every pixel. Legacy automatic profiles retain their exact palette.
		unique := colors[:0]
		seen := map[color.RGBA]bool{}
		for _, p := range colors {
			pixel := palettePixel(p)
			if !seen[pixel] {
				seen[pixel] = true
				unique = append(unique, p)
			}
		}
		colors = unique
	}
	if len(colors) == 0 || (opts.PaletteFile != "" && len(colors) < 2) {
		return nil, fmt.Errorf("в палитре не осталось подходящих цветов; выберите блоки или отключите монохромный режим")
	}
	return colors, nil
}

func exportPalette(path string, colors []MapColor) error {
	entries := make([]string, len(colors))
	for i, p := range colors {
		entries[i] = fmt.Sprintf("#%02X%02X%02X", int(math.Round(p.R)), int(math.Round(p.G)), int(math.Round(p.B)))
	}
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0644)
}

func RenderPreview(img image.Image, opts RenderOptions) (*PreviewResult, error) {
	if err := ValidateOptions(opts); err != nil {
		return nil, err
	}
	colors, err := paletteForOptions(opts)
	if err != nil {
		return nil, err
	}
	prepared, err := PrepareImage(img, opts)
	if err != nil {
		return nil, err
	}
	original := prepared
	if opts.Brightness != 0 || opts.Contrast != 1 || opts.Saturation != 1 || opts.Gamma != 1 || opts.Temperature != 0 || opts.Sharpen != 0 || opts.Denoise != 0 || opts.AutoContrast {
		plain := opts
		plain.Brightness, plain.Temperature, plain.Sharpen, plain.Denoise = 0, 0, 0, 0
		plain.Contrast, plain.Saturation, plain.Gamma, plain.AutoContrast = 1, 1, 1, false
		original, err = PrepareImage(img, plain)
		if err != nil {
			return nil, err
		}
	}
	background, smooth := detectBackgroundWithOptions(prepared, opts)
	for _, mask := range []struct {
		path     string
		suppress bool
	}{{opts.NoDitherMask, true}, {opts.DetailMask, false}} {
		if mask.path == "" {
			continue
		}
		decoded, err := decodeLocalImage(mask.path)
		if err != nil {
			return nil, fmt.Errorf("маска: %w", err)
		}
		selected, err := TransformMask(decoded, img.Bounds(), opts)
		if err != nil {
			return nil, fmt.Errorf("маска %s: %w", mask.path, err)
		}
		for i, marked := range selected {
			if marked {
				background[i] = mask.suppress
			}
		}
	}
	out, err := QuantizeImage(prepared, background, smooth, opts, colors)
	if err != nil {
		return nil, err
	}
	return &PreviewResult{Original: original, Prepared: prepared, Image: out, NoDitherMask: background, Palette: colors, BuildMode: opts.BuildMode}, nil
}

func processConfiguredImage(img image.Image, outPath string, opts RenderOptions, debug bool) error {
	result, err := RenderPreview(img, opts)
	if err != nil {
		return err
	}
	return savePreviewResult(result, outPath, opts, debug)
}

func savePreviewResult(result *PreviewResult, outPath string, opts RenderOptions, debug bool) error {
	if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
		return err
	}
	ext := filepath.Ext(outPath)
	base := strings.TrimSuffix(outPath, ext)
	if err := savePNG(base+"_original.png", result.Original); err != nil {
		return err
	}
	if err := savePNG(outPath, result.Image); err != nil {
		return err
	}
	if debug {
		if err := savePNG(base+"_prepared.png", result.Prepared); err != nil {
			return err
		}
		mask := image.NewGray(result.Image.Bounds())
		overlay := image.NewRGBA(result.Image.Bounds())
		width := result.Image.Bounds().Dx()
		for i, value := range result.NoDitherMask {
			x, y := i%width, i/width
			p := result.Prepared.RGBAAt(x, y)
			if value {
				mask.SetGray(x, y, color.Gray{255})
				p = color.RGBA{p.R / 2, uint8((int(p.G) + 255) / 2), p.B / 2, 255}
			}
			overlay.SetRGBA(x, y, p)
		}
		if err := savePNG(base+"_mask.png", mask); err != nil {
			return err
		}
		if err := savePNG(base+"_mask_overlay.png", overlay); err != nil {
			return err
		}
		if err := exportPalette(base+"_palette.json", result.Palette); err != nil {
			return err
		}
		if err := SaveOptions(base+"_settings.json", opts); err != nil {
			return err
		}
	}
	return nil
}
