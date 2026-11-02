package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"io"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
)

// RenderOptions contains the fully resolved settings for one conversion.
// Keep zero-valued fields in JSON: zero strength and false switches are useful
// explicit overrides when a configuration is layered over a preset.
type RenderOptions struct {
	MapsX               int     `json:"maps_x"`
	MapsY               int     `json:"maps_y"`
	Threshold           float64 `json:"threshold"`
	Dither              string  `json:"dither"`
	DitherStrength      float64 `json:"dither_strength"`
	ColorMetric         string  `json:"color_metric"`
	Background          string  `json:"background"`
	BackgroundColor     string  `json:"background_color"`
	BackgroundTolerance float64 `json:"background_tolerance"`
	SkyCleanup          bool    `json:"sky_cleanup"`
	CleanHaze           bool    `json:"clean_haze"`
	Brightness          float64 `json:"brightness"`
	Contrast            float64 `json:"contrast"`
	Saturation          float64 `json:"saturation"`
	Gamma               float64 `json:"gamma"`
	Temperature         float64 `json:"temperature"`
	Sharpen             float64 `json:"sharpen"`
	Denoise             float64 `json:"denoise"`
	AutoContrast        bool    `json:"auto_contrast"`
	Fit                 string  `json:"fit"`
	Resample            string  `json:"resample"`
	Matte               string  `json:"matte"`
	Crop                string  `json:"crop"`
	PaletteFile         string  `json:"palette_file"`
	Terracotta          bool    `json:"terracotta"`
	NaturalBlocks       bool    `json:"natural_blocks"`
	AllBlocks           bool    `json:"all_blocks"`
	SelectedBlocks      string  `json:"selected_blocks"`
	BuildMode           string  `json:"build_mode"`
	Rotation            int     `json:"rotation"`
	FlipHorizontal      bool    `json:"flip_horizontal"`
	FlipVertical        bool    `json:"flip_vertical"`
	Monochrome          bool    `json:"monochrome"`
	NoDitherMask        string  `json:"no_dither_mask"`
	DetailMask          string  `json:"detail_mask"`
}

func DefaultRenderOptions() RenderOptions {
	return RenderOptions{
		MapsX: 5, MapsY: 3, Threshold: 5,
		Dither: "fs", DitherStrength: 0.65, ColorMetric: "lab",
		Background: "auto", BackgroundTolerance: 45, SkyCleanup: true,
		Contrast: 1, Saturation: 1, Gamma: 1,
		Fit: "stretch", Resample: "bicubic", Matte: "#ffffff",
	}
}

func PresetNames() []string {
	return []string{"balanced", "portrait", "pastel", "landscape", "night", "lineart", "pixel", "flat", "clean-sky"}
}

func preservePresetAssets(preset, current RenderOptions) RenderOptions {
	preset.MapsX, preset.MapsY, preset.Fit, preset.Crop, preset.Matte = current.MapsX, current.MapsY, current.Fit, current.Crop, current.Matte
	preset.PaletteFile, preset.NoDitherMask, preset.DetailMask = current.PaletteFile, current.NoDitherMask, current.DetailMask
	preset.Terracotta = current.Terracotta
	preset.NaturalBlocks = current.NaturalBlocks
	preset.AllBlocks = current.AllBlocks
	preset.SelectedBlocks, preset.BuildMode = current.SelectedBlocks, current.BuildMode
	preset.Rotation, preset.FlipHorizontal, preset.FlipVertical = current.Rotation, current.FlipHorizontal, current.FlipVertical
	return preset
}

func PresetDescription(name string) string {
	switch name {
	case "balanced":
		return "Текущая обработка по умолчанию: Lab, умеренный дизеринг и автоматический фон."
	case "portrait":
		return "Мягкое смешивание оттенков портрета без автоматического отключения дизеринга на фоне."
	case "pastel":
		return "RGB и полное смешивание для пастельных переходов; может добавить зерно."
	case "landscape":
		return "RGB-смешивание для естественных цветов пейзажа без усиления резкости. Добавьте природные блоки для близких оттенков."
	case "night":
		return "Небольшое осветление теней и смешивание градиентов в тёмных сценах."
	case "lineart":
		return "Чёрно-белая палитра, автоматический контраст и чёткие линии без дизеринга."
	case "pixel":
		return "Ближайший сосед и отсутствие дизеринга для исходного пиксель-арта."
	case "flat":
		return "Автоматический ровный фон и умеренное зерно для иллюстраций с плоскими заливками."
	case "clean-sky":
		return "Очистка светлого неба и лишних цветных точек в дымке; проверьте бледные детали."
	default:
		return ""
	}
}

func PresetOptions(name string) (RenderOptions, error) {
	opts := DefaultRenderOptions()
	switch name {
	case "balanced":
	case "portrait":
		opts.Background, opts.SkyCleanup = "off", false
		opts.Threshold, opts.DitherStrength = 2, 0.8
	case "pastel":
		opts.Background, opts.SkyCleanup = "off", false
		opts.ColorMetric, opts.Threshold, opts.DitherStrength = "rgb", 0, 1
	case "landscape":
		opts.Background, opts.SkyCleanup = "off", false
		opts.ColorMetric, opts.Threshold, opts.DitherStrength = "rgb", 1, 1
	case "night":
		opts.Background, opts.SkyCleanup = "off", false
		opts.Threshold, opts.DitherStrength, opts.Gamma = 2, 0.8, 1.08
	case "lineart":
		opts.Monochrome, opts.AutoContrast = true, true
		opts.Dither, opts.Background, opts.SkyCleanup = "none", "off", false
	case "pixel":
		opts.Resample, opts.Dither = "nearest", "none"
		opts.Background, opts.SkyCleanup = "off", false
	case "flat":
		opts.DitherStrength = 0.45
	case "clean-sky":
		opts.CleanHaze = true
	default:
		return RenderOptions{}, fmt.Errorf("неизвестный пресет %q; доступны: %s", name, strings.Join(PresetNames(), ", "))
	}
	return opts, nil
}

// ParseHexColor accepts an opaque #RRGGBB color.
func ParseHexColor(value string) (color.RGBA, error) {
	if len(value) != 7 || value[0] != '#' {
		return color.RGBA{}, fmt.Errorf("цвет %q должен иметь формат #RRGGBB", value)
	}
	v, err := strconv.ParseUint(value[1:], 16, 24)
	if err != nil {
		return color.RGBA{}, fmt.Errorf("цвет %q должен иметь формат #RRGGBB", value)
	}
	return color.RGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 255}, nil
}

// ParseCrop uses source-image coordinates. An empty value means no cropping;
// whether a nonempty rectangle fits the source is checked by the image loader.
func ParseCrop(value string) (image.Rectangle, error) {
	if value == "" {
		return image.Rectangle{}, nil
	}
	parts := strings.Split(value, ",")
	if len(parts) != 4 {
		return image.Rectangle{}, fmt.Errorf("crop должен иметь формат x,y,w,h")
	}
	var v [4]int
	for i, part := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || n < 0 || (i >= 2 && n == 0) {
			return image.Rectangle{}, fmt.Errorf("crop: x и y должны быть неотрицательными целыми, w и h — положительными")
		}
		v[i] = n
	}
	maxInt := int(^uint(0) >> 1)
	if v[0] > maxInt-v[2] || v[1] > maxInt-v[3] {
		return image.Rectangle{}, fmt.Errorf("crop: координаты слишком велики")
	}
	return image.Rect(v[0], v[1], v[0]+v[2], v[1]+v[3]), nil
}

func ValidateOptions(opts RenderOptions) error {
	if opts.AllBlocks && opts.PaletteFile != "" {
		return fmt.Errorf("полная палитра использует блоки Minecraft; уберите свою RGB-палитру")
	}
	if opts.NaturalBlocks && opts.PaletteFile != "" {
		return fmt.Errorf("природные блоки расширяют встроенную палитру; уберите свою RGB-палитру")
	}
	if opts.Terracotta && opts.PaletteFile != "" {
		return fmt.Errorf("terracotta расширяет встроенную палитру; уберите свою RGB-палитру или отключите terracotta")
	}
	if opts.MapsX <= 0 || opts.MapsY <= 0 || opts.MapsX > 256 || opts.MapsY > 256 || opts.MapsX*opts.MapsY > 256 {
		return fmt.Errorf("размер должен быть положительным и не больше 256 карт суммарно")
	}
	for _, check := range []struct {
		name        string
		v, min, max float64
	}{
		{"threshold", opts.Threshold, 0, math.MaxFloat64},
		{"dither_strength", opts.DitherStrength, 0, 1},
		{"background_tolerance", opts.BackgroundTolerance, 0, 150},
		{"brightness", opts.Brightness, -100, 100},
		{"contrast", opts.Contrast, 0.1, 3},
		{"saturation", opts.Saturation, 0, 3},
		{"gamma", opts.Gamma, 0.2, 3},
		{"temperature", opts.Temperature, -100, 100},
		{"sharpen", opts.Sharpen, 0, 2},
		{"denoise", opts.Denoise, 0, 1},
	} {
		if math.IsNaN(check.v) || math.IsInf(check.v, 0) || check.v < check.min || check.v > check.max {
			return fmt.Errorf("%s: требуется конечное число в диапазоне [%g, %g]", check.name, check.min, check.max)
		}
	}
	for _, check := range []struct {
		name, value string
		allowed     []string
	}{
		{"dither", opts.Dither, []string{"fs", "ordered", "none"}},
		{"color_metric", opts.ColorMetric, []string{"lab", "rgb"}},
		{"background", opts.Background, []string{"auto", "off", "color"}},
		{"fit", opts.Fit, []string{"stretch", "contain", "cover"}},
		{"resample", opts.Resample, []string{"nearest", "bilinear", "bicubic"}},
	} {
		found := false
		for _, value := range check.allowed {
			found = found || check.value == value
		}
		if !found {
			return fmt.Errorf("%s: значение %q неизвестно; доступны: %s", check.name, check.value, strings.Join(check.allowed, ", "))
		}
	}
	if _, err := ParseHexColor(opts.Matte); err != nil {
		return fmt.Errorf("matte: %w", err)
	}
	if opts.Background == "color" || opts.BackgroundColor != "" {
		if _, err := ParseHexColor(opts.BackgroundColor); err != nil {
			return fmt.Errorf("background_color: %w", err)
		}
	}
	if _, err := ParseCrop(opts.Crop); err != nil {
		return err
	}
	if opts.BuildMode != "" && opts.BuildMode != "2d" && opts.BuildMode != "3d" {
		return fmt.Errorf("build_mode: выберите 2d или 3d")
	}
	if opts.Rotation != 0 && opts.Rotation != 90 && opts.Rotation != 180 && opts.Rotation != 270 {
		return fmt.Errorf("rotation: выберите 0, 90, 180 или 270 градусов")
	}
	if _, err := selectedBlockIDs(opts.SelectedBlocks); err != nil {
		return err
	}
	if opts.PaletteFile != "" && (opts.SelectedBlocks != "" || opts.BuildMode == "2d") {
		return fmt.Errorf("выбор блоков и режим 2D требуют встроенную палитру; очистите поле своей RGB-палитры")
	}
	for _, path := range []struct{ name, value string }{
		{"palette_file", opts.PaletteFile}, {"no_dither_mask", opts.NoDitherMask}, {"detail_mask", opts.DetailMask},
	} {
		if strings.ContainsRune(path.value, '\x00') {
			return fmt.Errorf("%s: путь содержит недопустимый нулевой символ", path.name)
		}
	}
	return nil
}

func (opts RenderOptions) Validate() error {
	return ValidateOptions(opts)
}

// LoadOptions overlays an object onto base, preserving explicit zeros and false.
// Paths present in that object are relative to its own file, not the process CWD.
func LoadOptions(path string, base RenderOptions) (RenderOptions, error) {
	opts, err := loadOptionsOverlay(path, base)
	if err != nil {
		return RenderOptions{}, err
	}
	if err := ValidateOptions(opts); err != nil {
		return RenderOptions{}, err
	}
	return opts, nil
}

// loadOptionsOverlay checks JSON syntax, field names, and types, while deferring
// value validation until all CLI overrides have been applied. For example, a
// profile can select a color background while the CLI supplies its actual color.
func loadOptionsOverlay(path string, base RenderOptions) (RenderOptions, error) {
	f, err := os.Open(path)
	if err != nil {
		return RenderOptions{}, fmt.Errorf("чтение конфигурации: %w", err)
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	first, err := dec.Token()
	if err != nil || first != json.Delim('{') {
		return RenderOptions{}, fmt.Errorf("конфигурация должна быть JSON-объектом")
	}
	known := make(map[string]bool)
	typ := reflect.TypeOf(RenderOptions{})
	for i := 0; i < typ.NumField(); i++ {
		known[typ.Field(i).Tag.Get("json")] = true
	}
	fields := make(map[string]json.RawMessage)
	for dec.More() {
		token, err := dec.Token()
		if err != nil {
			return RenderOptions{}, fmt.Errorf("конфигурация: %w", err)
		}
		key, ok := token.(string)
		if !ok || !known[key] {
			return RenderOptions{}, fmt.Errorf("неизвестное поле конфигурации %q", token)
		}
		if _, exists := fields[key]; exists {
			return RenderOptions{}, fmt.Errorf("повторяющееся поле конфигурации %q", key)
		}
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return RenderOptions{}, fmt.Errorf("конфигурация, поле %s: %w", key, err)
		}
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return RenderOptions{}, fmt.Errorf("конфигурация, поле %s: null не является значением настройки", key)
		}
		fields[key] = raw
	}
	if _, err := dec.Token(); err != nil {
		return RenderOptions{}, fmt.Errorf("конфигурация: %w", err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return RenderOptions{}, fmt.Errorf("после JSON-объекта конфигурации обнаружены лишние данные")
	}
	data, err := json.Marshal(fields)
	if err != nil {
		return RenderOptions{}, fmt.Errorf("конфигурация: %w", err)
	}
	opts := base
	if err := json.Unmarshal(data, &opts); err != nil {
		return RenderOptions{}, fmt.Errorf("конфигурация: %w", err)
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return RenderOptions{}, fmt.Errorf("путь конфигурации: %w", err)
	}
	for _, item := range []struct {
		key   string
		value *string
	}{
		{"palette_file", &opts.PaletteFile}, {"no_dither_mask", &opts.NoDitherMask}, {"detail_mask", &opts.DetailMask},
	} {
		if _, present := fields[item.key]; present && *item.value != "" {
			if !filepath.IsAbs(*item.value) {
				*item.value = filepath.Join(filepath.Dir(absPath), *item.value)
			} else {
				*item.value = filepath.Clean(*item.value)
			}
		}
	}
	return opts, nil
}

func SaveOptions(path string, opts RenderOptions) error {
	if err := ValidateOptions(opts); err != nil {
		return err
	}
	// Absolute asset paths make a saved configuration portable between working
	// directories without silently changing which local mask or palette it uses.
	for _, value := range []*string{&opts.PaletteFile, &opts.NoDitherMask, &opts.DetailMask} {
		if *value != "" {
			abs, err := filepath.Abs(*value)
			if err != nil {
				return fmt.Errorf("путь ресурса конфигурации: %w", err)
			}
			*value = abs
		}
	}
	data, err := json.MarshalIndent(opts, "", "  ")
	if err != nil {
		return fmt.Errorf("сохранение конфигурации: %w", err)
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("создание каталога конфигурации: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("сохранение конфигурации: %w", err)
	}
	return nil
}
