package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html/template"
	"image"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type cliRequest struct {
	options                                                                                     RenderOptions
	input, directory, links, output, outDir, preset, config, saveConfig, exportPalette, compare string
	debug, listPresets, showConfig                                                              bool
	resolve                                                                                     func(string) (RenderOptions, error)
	gui                                                                                         bool
	nbt, nbtTiles                                                                               string
	build                                                                                       BuildOptions
	qr                                                                                          QRCodeOptions
}

func parseCLI(args []string, writer io.Writer) (cliRequest, error) {
	var req cliRequest
	cli := DefaultRenderOptions()
	req.build = DefaultBuildOptions()
	req.qr = DefaultQRCodeOptions()
	fs := flag.NewFlagSet("mapart", flag.ContinueOnError)
	fs.SetOutput(writer)
	fs.Usage = func() {
		fmt.Fprintln(writer, "Мап арт: mapart -i picture.png -preset portrait -x 5 -y 3\nПриоритет настроек: пресет → JSON-профиль → явно указанные параметры.")
		fs.PrintDefaults()
	}
	fs.StringVar(&req.input, "i", "", "Исходная картинка")
	fs.StringVar(&req.qr.Text, "qr", "", "Создать QR-код из ссылки или текста вместо картинки")
	fs.StringVar(&req.qr.Style, "qr-style", req.qr.Style, "Элементы QR: square, rounded, dots")
	fs.StringVar(&req.qr.ForegroundBlock, "qr-fg", req.qr.ForegroundBlock, "Блок элементов QR, например black_wool")
	fs.StringVar(&req.qr.BackgroundBlock, "qr-bg", req.qr.BackgroundBlock, "Блок фона QR, например white_wool")
	fs.StringVar(&req.qr.FinderBlock, "qr-finder", req.qr.FinderBlock, "Блок углов QR")
	fs.StringVar(&req.qr.LogoPath, "qr-logo", "", "Картинка логотипа для центра QR (необязательно)")
	fs.IntVar(&req.qr.LogoPercent, "qr-logo-size", req.qr.LogoPercent, "Размер логотипа: 5..20 процентов ширины QR")
	fs.BoolVar(&req.gui, "gui", false, "Открыть графическое окно")
	fs.StringVar(&req.nbt, "nbt", "", "Сохранить схему .nbt для Litematica: 2D или 3D (с -i или -qr)")
	fs.StringVar(&req.nbtTiles, "nbt-tiles", "", "ZIP с отдельной NBT для каждой карты 128x128 (с -i или -qr)")
	fs.BoolVar(&req.build.Supports, "supports", true, "Добавлять опоры из булыжника под перепадами высоты")
	fs.IntVar(&req.build.MaxHeight, "max-height", req.build.MaxHeight, "Максимальная высота схемы в блоках")
	fs.IntVar(&req.build.DataVersion, "data-version", req.build.DataVersion, "NBT DataVersion: по умолчанию 5023 (Java 26.3)")
	fs.StringVar(&req.directory, "dir", "", "Папка с картинками")
	fs.StringVar(&req.links, "links", "", "TXT со ссылками, можно URL X Y")
	fs.StringVar(&req.output, "o", "", "Путь к итоговому PNG для одной картинки")
	fs.StringVar(&req.outDir, "out-dir", "output", "Папка результатов")
	fs.StringVar(&req.preset, "preset", "balanced", "Пресет; варианты: -list-presets")
	fs.StringVar(&req.config, "config", "", "JSON-профиль настроек")
	fs.StringVar(&req.saveConfig, "save-config", "", "Сохранить итоговые настройки в JSON")
	fs.StringVar(&req.exportPalette, "export-palette", "", "Сохранить используемую палитру в JSON")
	fs.StringVar(&req.compare, "compare", "", "Сравнить пресеты через запятую для -i; либо all")
	fs.BoolVar(&req.debug, "debug", false, "Сохранить маску, её наложение, подготовленное изображение и настройки")
	fs.BoolVar(&req.listPresets, "list-presets", false, "Показать доступные пресеты")
	fs.BoolVar(&req.showConfig, "show-config", false, "Показать итоговые параметры JSON")
	apply := map[string]func(*RenderOptions){}
	bindInt := func(name string, target *int, set func(*RenderOptions, int), help string) {
		fs.IntVar(target, name, *target, help)
		apply[name] = func(o *RenderOptions) { set(o, *target) }
	}
	bindFloat := func(name string, target *float64, set func(*RenderOptions, float64), help string) {
		fs.Float64Var(target, name, *target, help)
		apply[name] = func(o *RenderOptions) { set(o, *target) }
	}
	bindString := func(name string, target *string, set func(*RenderOptions, string), help string) {
		fs.StringVar(target, name, *target, help)
		apply[name] = func(o *RenderOptions) { set(o, *target) }
	}
	bindBool := func(name string, target *bool, set func(*RenderOptions, bool), help string) {
		fs.BoolVar(target, name, *target, help)
		apply[name] = func(o *RenderOptions) { set(o, *target) }
	}
	bindInt("x", &cli.MapsX, func(o *RenderOptions, v int) { o.MapsX = v }, "Ширина в картах по 128 пикселей")
	bindInt("y", &cli.MapsY, func(o *RenderOptions, v int) { o.MapsY = v }, "Высота в картах по 128 пикселей")
	bindFloat("t", &cli.Threshold, func(o *RenderOptions, v float64) { o.Threshold = v }, "Порог дизеринга в Delta E; 0 для всех неточных оттенков")
	bindString("dither", &cli.Dither, func(o *RenderOptions, v string) { o.Dither = v }, "Дизеринг: fs, ordered, none")
	bindFloat("strength", &cli.DitherStrength, func(o *RenderOptions, v float64) { o.DitherStrength = v }, "Сила дизеринга: 0..1")
	bindString("metric", &cli.ColorMetric, func(o *RenderOptions, v string) { o.ColorMetric = v }, "Подбор цвета: lab или rgb")
	bindString("background", &cli.Background, func(o *RenderOptions, v string) { o.Background = v }, "Маска фона: auto, off, color")
	bindString("bg-color", &cli.BackgroundColor, func(o *RenderOptions, v string) { o.BackgroundColor = v }, "Цвет фона #RRGGBB для -background color")
	bindFloat("bg-tolerance", &cli.BackgroundTolerance, func(o *RenderOptions, v float64) { o.BackgroundTolerance = v }, "Допуск цвета маски фона: 0..150")
	bindBool("sky-cleanup", &cli.SkyCleanup, func(o *RenderOptions, v bool) { o.SkyCleanup = v }, "Маска светлого неба; отключение: -sky-cleanup=false")
	bindBool("clean-haze", &cli.CleanHaze, func(o *RenderOptions, v bool) { o.CleanHaze = v }, "Приглушать яркие точки в светлой дымке (ослабляет пастель)")
	bindFloat("brightness", &cli.Brightness, func(o *RenderOptions, v float64) { o.Brightness = v }, "Яркость: -100..100, 0 без изменения")
	bindFloat("contrast", &cli.Contrast, func(o *RenderOptions, v float64) { o.Contrast = v }, "Контраст: 0.1..3, 1 без изменения")
	bindFloat("saturation", &cli.Saturation, func(o *RenderOptions, v float64) { o.Saturation = v }, "Насыщенность: 0..3, 1 без изменения")
	bindFloat("gamma", &cli.Gamma, func(o *RenderOptions, v float64) { o.Gamma = v }, "Гамма: 0.2..3; больше 1 осветляет полутона")
	bindFloat("temperature", &cli.Temperature, func(o *RenderOptions, v float64) { o.Temperature = v }, "Теплее/холоднее: -100..100")
	bindFloat("sharpen", &cli.Sharpen, func(o *RenderOptions, v float64) { o.Sharpen = v }, "Резкость: 0..2")
	bindFloat("denoise", &cli.Denoise, func(o *RenderOptions, v float64) { o.Denoise = v }, "Подавление исходного мелкого шума: 0..1")
	bindBool("auto-contrast", &cli.AutoContrast, func(o *RenderOptions, v bool) { o.AutoContrast = v }, "Растянуть тональный диапазон по процентилям")
	bindString("fit", &cli.Fit, func(o *RenderOptions, v string) { o.Fit = v }, "Вписывание: stretch, contain (поля), cover (обрезка)")
	bindString("resample", &cli.Resample, func(o *RenderOptions, v string) { o.Resample = v }, "Масштабирование: nearest, bilinear, bicubic")
	bindString("matte", &cli.Matte, func(o *RenderOptions, v string) { o.Matte = v }, "Цвет прозрачности и полей #RRGGBB")
	bindString("crop", &cli.Crop, func(o *RenderOptions, v string) { o.Crop = v }, "Обрезка исходника: x,y,width,height")
	bindString("palette", &cli.PaletteFile, func(o *RenderOptions, v string) { o.PaletteFile = v }, "Своя палитра: JSON-массив цветов #RRGGBB")
	bindBool("terracotta", &cli.Terracotta, func(o *RenderOptions, v bool) { o.Terracotta = v }, "Добавить 48 оттенков терракоты к шерсти (96 всего, PNG и NBT)")
	bindBool("natural-blocks", &cli.NaturalBlocks, func(o *RenderOptions, v bool) { o.NaturalBlocks = v }, "Добавить глину, лёд, песчаник, камень, дубовые доски и призмариновые кирпичи (18 оттенков, PNG и NBT)")
	bindBool("all-blocks", &cli.AllBlocks, func(o *RenderOptions, v bool) { o.AllBlocks = v }, "Полная палитра без дублей: 60 материалов, 60 цветов в 2D и 180 оттенков в 3D (Java 26.3)")
	bindString("blocks", &cli.SelectedBlocks, func(o *RenderOptions, v string) { o.SelectedBlocks = v }, "Разрешённые блоки через запятую, например white_wool,black_wool,red_terracotta")
	bindString("build-mode", &cli.BuildMode, func(o *RenderOptions, v string) { o.BuildMode = v }, "Постройка: 3d (лестница, по умолчанию) или 2d (плоская)")
	bindInt("rotate", &cli.Rotation, func(o *RenderOptions, v int) { o.Rotation = v }, "Поворот исходника по часовой стрелке: 0, 90, 180, 270")
	bindBool("flip-horizontal", &cli.FlipHorizontal, func(o *RenderOptions, v bool) { o.FlipHorizontal = v }, "Отразить исходник по горизонтали после поворота")
	bindBool("flip-vertical", &cli.FlipVertical, func(o *RenderOptions, v bool) { o.FlipVertical = v }, "Отразить исходник по вертикали после поворота")
	bindBool("monochrome", &cli.Monochrome, func(o *RenderOptions, v bool) { o.Monochrome = v }, "Использовать только нейтральные цвета палитры")
	bindString("no-dither-mask", &cli.NoDitherMask, func(o *RenderOptions, v string) { o.NoDitherMask = v }, "Белым отметить области без дизеринга в PNG-маске")
	bindString("detail-mask", &cli.DetailMask, func(o *RenderOptions, v string) { o.DetailMask = v }, "Белым защитить детали от автоматических масок")
	if err := fs.Parse(args); err != nil {
		return req, err
	}
	if fs.NArg() != 0 {
		return req, fmt.Errorf("неожиданные аргументы: %s; путь задаётся через -i", strings.Join(fs.Args(), " "))
	}
	visited := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { visited[f.Name] = true })
	req.resolve = func(preset string) (RenderOptions, error) {
		opts, err := PresetOptions(preset)
		if err != nil {
			return opts, err
		}
		if req.config != "" {
			opts, err = loadOptionsOverlay(req.config, opts)
			if err != nil {
				return opts, err
			}
		}
		for name, set := range apply {
			if visited[name] {
				set(&opts)
			}
		}
		return opts, ValidateOptions(opts)
	}
	var err error
	req.options, err = req.resolve(req.preset)
	if err == nil && req.qr.Text != "" {
		if req.options.PaletteFile != "" {
			return req, fmt.Errorf("QR-код использует блоки Minecraft; уберите -palette")
		}
		if visited["x"] || req.config != "" {
			req.qr.MapsX = req.options.MapsX
		}
		if visited["y"] || req.config != "" {
			req.qr.MapsY = req.options.MapsY
		}
		if req.options.BuildMode != "" {
			req.qr.BuildMode = req.options.BuildMode
		}
		req.options = QRRenderOptions(req.qr)
		err = ValidateOptions(req.options)
	}
	return req, err
}

func runCLI(args []string) error {
	req, err := parseCLI(args, os.Stdout)
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	if err != nil {
		return err
	}
	if req.gui {
		if err := req.build.Validate(); err != nil {
			return err
		}
		if req.directory != "" || req.links != "" || req.compare != "" || req.nbt != "" || req.nbtTiles != "" || req.output != "" || req.qr.Text != "" {
			return fmt.Errorf("-gui принимает -i, пресет и настройки; генератор QR и сохранение выбираются в окне")
		}
		return runGUI(req.options, req.input, req.build)
	}
	if req.nbt != "" || req.nbtTiles != "" {
		if (req.input == "" && req.qr.Text == "") || req.compare != "" || req.listPresets {
			return fmt.Errorf("-nbt и -nbt-tiles работают с одним -i или -qr, без -compare и -list-presets")
		}
		if err := req.build.Validate(); err != nil {
			return err
		}
		if req.options.PaletteFile != "" {
			return fmt.Errorf("NBT требует встроенную палитру блоков; уберите -palette")
		}
		if req.nbt != "" && !strings.EqualFold(filepath.Ext(req.nbt), ".nbt") {
			return fmt.Errorf("-nbt должен указывать файл .nbt")
		}
		if req.nbtTiles != "" && !strings.EqualFold(filepath.Ext(req.nbtTiles), ".zip") {
			return fmt.Errorf("-nbt-tiles должен указывать файл .zip")
		}
	}
	// Reject incompatible commands before printing or writing any requested
	// profiles or palette exports. Informational actions need no source, but
	// they do not make otherwise invalid -o/-compare combinations meaningful.
	modes := 0
	for _, v := range []string{req.input, req.directory, req.links, req.qr.Text} {
		if v != "" {
			modes++
		}
	}
	if req.compare != "" && (req.input == "" || req.output != "" || req.qr.Text != "") {
		return fmt.Errorf("-compare работает с -i и -out-dir; -o задаёт только один PNG")
	}
	if req.output != "" && req.input == "" && req.qr.Text == "" {
		return fmt.Errorf("-o работает с -i или -qr; для папки или ссылок используйте -out-dir")
	}
	if ext := filepath.Ext(req.output); ext != "" && !strings.EqualFold(ext, ".png") {
		return fmt.Errorf("-o должен указывать файл PNG")
	}
	metadataOnly := req.listPresets || req.saveConfig != "" || req.showConfig || req.exportPalette != ""
	if modes > 1 || (modes == 0 && !metadataOnly) {
		return fmt.Errorf("укажите один источник: -i, -dir, -links или -qr")
	}
	if req.listPresets {
		for _, name := range PresetNames() {
			fmt.Printf("%-12s %s\n", name, PresetDescription(name))
		}
		return nil
	}
	opts := req.options
	if req.showConfig {
		data, _ := json.MarshalIndent(opts, "", "  ")
		fmt.Println(string(data))
	}
	if req.saveConfig != "" {
		if err := SaveOptions(req.saveConfig, opts); err != nil {
			return err
		}
	}
	if req.exportPalette != "" {
		colors, err := paletteForOptions(opts)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(req.exportPalette), 0755); err != nil {
			return err
		}
		if err := exportPalette(req.exportPalette, colors); err != nil {
			return err
		}
	}
	if modes == 0 {
		return nil
	}
	if req.input != "" || req.qr.Text != "" {
		var result *PreviewResult
		if req.qr.Text != "" {
			generated, err := GenerateQRCode(req.qr)
			if err != nil {
				return err
			}
			result, opts = generated.Preview, generated.Options
			fmt.Printf("QR-код проверен: %d × %d элементов, %d блока на элемент\n", generated.Modules, generated.Modules, generated.ModulePixels)
		} else {
			img, err := decodeLocalImage(req.input)
			if err != nil {
				return err
			}
			if req.compare != "" {
				return comparePresets(img, req)
			}
			result, err = RenderPreview(img, opts)
			if err != nil {
				return err
			}
		}
		out := req.output
		if out == "" {
			out = filepath.Join(req.outDir, filepath.Base(req.input)+".png")
			if req.qr.Text != "" {
				out = filepath.Join(req.outDir, "qr-code.png")
			}
		}
		if filepath.Ext(out) == "" {
			out += ".png"
		}
		if req.nbt != "" {
			s, err := BuildStructure(result, req.build)
			if err != nil {
				return err
			}
			if err = ExportStructure(req.nbt, s); err != nil {
				return err
			}
			fmt.Printf("NBT: %s — %d × %d × %d, %d блоков\n", req.nbt, s.Info.Size[0], s.Info.Size[1], s.Info.Size[2], s.Info.TotalBlocks)
		}
		if req.nbtTiles != "" {
			if err := ExportStructureTiles(req.nbtTiles, result, req.build); err != nil {
				return err
			}
			fmt.Println("Карты NBT:", req.nbtTiles)
		}
		if err := savePreviewResult(result, out, opts, req.debug); err != nil {
			return err
		}
		fmt.Println("Готово:", out)
		return nil
	}
	failed := 0
	if req.directory != "" {
		files, err := os.ReadDir(req.directory)
		if err != nil {
			return err
		}
		count := 0
		for _, file := range files {
			if file.IsDir() || !supportedImagePath(file.Name()) {
				continue
			}
			count++
			path := filepath.Join(req.directory, file.Name())
			img, err := decodeLocalImage(path)
			if err == nil {
				err = processConfiguredImage(img, filepath.Join(req.outDir, file.Name()+".png"), opts, req.debug)
			}
			if err != nil {
				failed++
				fmt.Fprintln(os.Stderr, path, err)
			} else {
				fmt.Println("Готово:", file.Name())
			}
		}
		if count == 0 {
			return fmt.Errorf("в папке не найдено поддерживаемых изображений")
		}
	} else {
		f, err := os.Open(req.links)
		if err != nil {
			return err
		}
		defer f.Close()
		client := &http.Client{Timeout: 30 * time.Second}
		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 4096), 1024*1024)
		index := 0
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			index++
			parts := strings.Fields(line)
			current := opts
			if len(parts) != 1 && len(parts) != 3 {
				failed++
				fmt.Fprintln(os.Stderr, "Строка", index, "ожидает URL либо URL X Y")
				continue
			}
			if len(parts) == 3 {
				var ex, ey error
				current.MapsX, ex = strconv.Atoi(parts[1])
				current.MapsY, ey = strconv.Atoi(parts[2])
				if ex != nil || ey != nil || ValidateOptions(current) != nil {
					failed++
					fmt.Fprintln(os.Stderr, "Некорректный размер в строке", index)
					continue
				}
			}
			request, err := http.NewRequest("GET", parts[0], nil)
			if err != nil {
				failed++
				fmt.Fprintln(os.Stderr, err)
				continue
			}
			request.Header.Set("User-Agent", "MapArtPreview/1.0")
			response, err := client.Do(request)
			if err != nil {
				failed++
				fmt.Fprintln(os.Stderr, err)
				continue
			}
			if response.StatusCode != http.StatusOK {
				response.Body.Close()
				failed++
				fmt.Fprintln(os.Stderr, "HTTP", response.StatusCode, parts[0])
				continue
			}
			img, _, err := image.Decode(response.Body)
			response.Body.Close()
			if err == nil {
				err = processConfiguredImage(img, filepath.Join(req.outDir, fmt.Sprintf("url_image_%d.png", index)), current, req.debug)
			}
			if err != nil {
				failed++
				fmt.Fprintln(os.Stderr, parts[0], err)
			} else {
				fmt.Println("Готово:", parts[0])
			}
		}
		if err := scanner.Err(); err != nil {
			return err
		}
		if index == 0 {
			return fmt.Errorf("в файле нет ссылок")
		}
	}
	if failed > 0 {
		return fmt.Errorf("не удалось обработать %d изображений; остальные результаты сохранены", failed)
	}
	return nil
}

func supportedImagePath(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png", ".jpg", ".jpeg", ".webp", ".gif", ".bmp", ".tif", ".tiff":
		return true
	}
	return false
}

func comparePresets(img image.Image, req cliRequest) error {
	names := strings.Split(req.compare, ",")
	if req.compare == "all" {
		names = PresetNames()
	}
	type card struct{ Name, Description, Original, Render, Config string }
	var cards []card
	seen := map[string]bool{}
	stem := strings.TrimSuffix(filepath.Base(req.input), filepath.Ext(req.input))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if seen[name] {
			continue
		}
		seen[name] = true
		opts, err := req.resolve(name)
		if err != nil {
			return err
		}
		file := stem + "__" + name + ".png"
		out := filepath.Join(req.outDir, file)
		if err := processConfiguredImage(img, out, opts, req.debug); err != nil {
			return err
		}
		profile := stem + "__" + name + ".json"
		if err := SaveOptions(filepath.Join(req.outDir, profile), opts); err != nil {
			return err
		}
		cards = append(cards, card{name, PresetDescription(name), strings.TrimSuffix(file, ".png") + "_original.png", file, profile})
		fmt.Println("Готово:", out)
	}
	page, err := template.New("comparison").Parse(`<!doctype html><html lang="ru"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Сравнение пресетов</title><style>body{font:16px system-ui;background:#121824;color:#e8edf7;margin:24px}main{display:grid;grid-template-columns:repeat(auto-fit,minmax(min(100%,620px),1fr));gap:20px}article{background:#202b3d;border-radius:12px;padding:18px}.pair{display:flex;gap:10px}.pair a{width:50%}img{width:100%;image-rendering:pixelated}a{color:#9acaff}h2{margin:0}p{color:#c1cada}</style><h1>Сравнение пресетов</h1><p>Слева исходник после вписывания, справа мап арт. Профиль сохраняет все фактические настройки, включая ваши параметры командной строки.</p><main>{{range .}}<article><h2>{{.Name}}</h2><p>{{.Description}}</p><div class="pair"><a href="{{.Original}}"><img src="{{.Original}}" loading="lazy" alt="Исходник"></a><a href="{{.Render}}"><img src="{{.Render}}" loading="lazy" alt="Мап арт"></a></div><p><a href="{{.Config}}">JSON-профиль</a> · <a href="{{.Render}}">PNG</a></p></article>{{end}}</main></html>`)
	if err != nil {
		return err
	}
	path := filepath.Join(req.outDir, stem+"__compare.html")
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	err = page.Execute(f, cards)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	fmt.Println("Сравнение:", path)
	return nil
}
