//go:build !windows && gui

package main

import (
	"fmt"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

func (g *FyneGUI) acceptQRCode(q QRCodeOptions, result *QRCodeResult, structure *Structure) {
	g.qrOptions, g.generatedSource = q, result.Source
	g.path.SetText(generatedQRSource)
	g.writeOptions(result.Options)
	g.showResult(result.Preview, result.Options, generatedQRSource, structure, nil)
	g.previewTabs.SelectIndex(1)
	g.updateButtons()
}

func (g *FyneGUI) qrGenerator() {
	if g.busy {
		return
	}
	q := g.qrOptions
	if q.BuildMode == "" {
		q = DefaultQRCodeOptions()
	}
	text := widget.NewMultiLineEntry()
	text.SetPlaceHolder("https://example.com или любой текст")
	text.SetMinRowsVisible(3)
	text.SetText(q.Text)
	mode := widget.NewSelect(qrModeLabels, nil)
	mode.SetSelectedIndex(0)
	if q.BuildMode == "3d" {
		mode.SetSelectedIndex(1)
	}
	style := widget.NewSelect(qrStyleLabels, nil)
	for i, value := range qrStyles {
		if value == q.Style {
			style.SetSelectedIndex(i)
		}
	}
	mapsX, mapsY := widget.NewEntry(), widget.NewEntry()
	mapsX.SetText(strconv.Itoa(q.MapsX))
	mapsY.SetText(strconv.Itoa(q.MapsY))
	foreground, background, finder := widget.NewSelect(qrBlockLabels(), nil), widget.NewSelect(qrBlockLabels(), nil), widget.NewSelect(qrBlockLabels(), nil)
	foreground.SetSelectedIndex(qrBlockIndex(q.ForegroundBlock))
	background.SetSelectedIndex(qrBlockIndex(q.BackgroundBlock))
	finder.SetSelectedIndex(qrBlockIndex(q.FinderBlock))
	logo, logoSize := widget.NewEntry(), widget.NewEntry()
	logo.SetPlaceHolder("Необязательно: картинка для центра QR")
	logo.SetText(q.LogoPath)
	logoSize.SetText(strconv.Itoa(q.LogoPercent))
	chooseLogo := widget.NewButton("Выбрать…", func() {
		g.openFile("Логотип в центре QR", []string{".png", ".jpg", ".jpeg", ".webp", ".gif", ".bmp", ".tif", ".tiff"}, logo.SetText)
	})
	clearLogo := widget.NewButton("Убрать", func() { logo.SetText("") })
	preview := canvas.NewImageFromImage(nil)
	preview.FillMode, preview.ScaleMode = canvas.ImageFillContain, canvas.ImageScalePixels
	preview.SetMinSize(fyne.NewSize(300, 300))
	info := wrapped("Вставьте ссылку и нажмите «Обновить превью» или «Создать мап-арт». Цвета показаны как на карте Minecraft.")
	var dlg *dialog.CustomDialog
	closed := false
	var update, create *widget.Button
	controls := []fyne.Disableable{text, mode, style, mapsX, mapsY, foreground, background, finder, logo, logoSize, chooseLogo, clearLogo}
	lock := func(busy bool) {
		for _, control := range controls {
			if busy {
				control.Disable()
			} else {
				control.Enable()
			}
		}
		if busy {
			update.Disable()
			create.Disable()
		} else {
			update.Enable()
			create.Enable()
		}
	}
	read := func() (QRCodeOptions, error) {
		value := q
		value.Text = text.Text
		value.BuildMode = "2d"
		if mode.SelectedIndex() == 1 {
			value.BuildMode = "3d"
		}
		if style.SelectedIndex() >= 0 {
			value.Style = qrStyles[style.SelectedIndex()]
		}
		var err error
		value.MapsX, err = strconv.Atoi(strings.TrimSpace(mapsX.Text))
		if err != nil {
			return value, fmt.Errorf("ширина: укажите целое число карт")
		}
		value.MapsY, err = strconv.Atoi(strings.TrimSpace(mapsY.Text))
		if err != nil {
			return value, fmt.Errorf("высота: укажите целое число карт")
		}
		value.LogoPercent, err = strconv.Atoi(strings.TrimSpace(logoSize.Text))
		if err != nil {
			return value, fmt.Errorf("размер логотипа: укажите число от 5 до 20%%")
		}
		value.ForegroundBlock, value.BackgroundBlock, value.FinderBlock = qrBlockName(foreground.SelectedIndex()), qrBlockName(background.SelectedIndex()), qrBlockName(finder.SelectedIndex())
		value.LogoPath = strings.TrimSpace(logo.Text)
		return value, nil
	}
	generate := func(apply bool) {
		if g.busy {
			return
		}
		value, err := read()
		if err != nil {
			info.SetText(err.Error())
			return
		}
		build, err := g.readBuild()
		if err != nil {
			info.SetText(err.Error())
			return
		}
		lock(true)
		info.SetText("Генерация и проверка QR-кода…")
		var result *QRCodeResult
		var structure *Structure
		// Handle errors inside the dialog, keeping the previous map untouched.
		var generationErr error
		g.startWork("Генерация QR-кода…", func() error {
			result, generationErr = GenerateQRCode(value)
			if generationErr == nil {
				structure, generationErr = BuildStructure(result.Preview, build)
			}
			return nil
		}, func() {
			if closed {
				return
			}
			lock(false)
			if generationErr != nil {
				info.SetText(generationErr.Error())
				g.status.SetText(generationErr.Error())
				return
			}
			preview.Image = structure.MapImage
			preview.Refresh()
			info.SetText(fmt.Sprintf("QR-код читается · %d × %d элементов · %d блока на элемент\n%d × %d карт · %s", result.Modules, result.Modules, result.ModulePixels, value.MapsX, value.MapsY, buildModeLabel(value.BuildMode)))
			g.status.SetText("QR-превью готово и проверено.")
			if apply {
				g.acceptQRCode(value, result, structure)
				dlg.Hide()
			}
		})
	}
	update = widget.NewButton("Обновить превью", func() { generate(false) })
	create = widget.NewButtonWithIcon("Создать мап-арт", theme.MediaPlayIcon(), func() { generate(true) })
	create.Importance = widget.HighImportance
	foregroundSwatch, backgroundSwatch, finderSwatch := canvas.NewRectangle(nil), canvas.NewRectangle(nil), canvas.NewRectangle(nil)
	refreshColours := func() {
		for i, selection := range []*widget.Select{foreground, background, finder} {
			shade := "Normal"
			if mode.SelectedIndex() == 1 {
				shade = "Dark"
				if i == 1 {
					shade = "Light"
				}
			}
			c, _ := qrBlockColour(qrBlockName(selection.SelectedIndex()), shade)
			swatch := []*canvas.Rectangle{foregroundSwatch, backgroundSwatch, finderSwatch}[i]
			swatch.FillColor = c
			swatch.SetMinSize(fyne.NewSize(28, 28))
			swatch.Refresh()
		}
	}
	changed := func() {
		info.SetText("Оформление изменено. Обновите превью или создайте мап-арт.")
		refreshColours()
	}
	for _, entry := range []*widget.Entry{text, mapsX, mapsY, logo, logoSize} {
		entry.OnChanged = func(string) { changed() }
	}
	for _, selection := range []*widget.Select{mode, style, foreground, background, finder} {
		selection.OnChanged = func(string) { changed() }
	}
	refreshColours()
	form := widget.NewForm(
		widget.NewFormItem("Ссылка / текст", text),
		widget.NewFormItem("Карт X × Y", container.NewGridWithColumns(2, mapsX, mapsY)),
		widget.NewFormItem("Постройка", mode),
		widget.NewFormItem("Элементы", style),
		widget.NewFormItem("Цвет QR", container.NewBorder(nil, nil, foregroundSwatch, nil, foreground)),
		widget.NewFormItem("Фон", container.NewBorder(nil, nil, backgroundSwatch, nil, background)),
		widget.NewFormItem("Углы QR", container.NewBorder(nil, nil, finderSwatch, nil, finder)),
		widget.NewFormItem("Логотип", container.NewBorder(nil, nil, nil, container.NewHBox(chooseLogo, clearLogo), logo)),
		widget.NewFormItem("Логотип, %", logoSize))
	left := container.NewBorder(nil, wrapped("Цвета выбираются из блоков Minecraft. Вокруг кода автоматически остаётся свободное поле. После создания доступны кисти и экспорт схемы."), nil, nil, container.NewVScroll(form))
	right := container.NewBorder(nil, info, nil, nil, preview)
	split := container.NewHSplit(left, right)
	split.Offset = .57
	content := container.NewBorder(nil, container.NewHBox(layout.NewSpacer(), update, create), nil, nil, split)
	dlg = dialog.NewCustom("Генератор QR-кода — мап-арт", "Закрыть", content, g.window)
	dlg.SetOnClosed(func() { closed = true })
	dlg.Resize(fyne.NewSize(1050, 710))
	dlg.Show()
}
