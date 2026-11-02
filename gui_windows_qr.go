//go:build windows

package main

import (
	"fmt"
	"strings"

	"github.com/lxn/walk"
	d "github.com/lxn/walk/declarative"
)

func (a *nativeApp) acceptQRCode(q QRCodeOptions, result *QRCodeResult, structure *Structure) {
	a.qrOptions, a.generatedSource = q, result.Source
	a.path.SetText(generatedQRSource)
	a.writeOptions(result.Options)
	a.showResult(result.Preview, result.Options, generatedQRSource, structure, nil)
	a.previewTabs.SetCurrentIndex(1)
}

func (a *nativeApp) qrGenerator() {
	if a.busy {
		return
	}
	q := a.qrOptions
	if q.BuildMode == "" {
		q = DefaultQRCodeOptions()
	}
	var dlg *walk.Dialog
	var cancel, update, create *walk.PushButton
	var text *walk.TextEdit
	var mapsX, mapsY, logoSize *walk.NumberEdit
	var mode, style, foreground, background, finder *walk.ComboBox
	var logo *walk.LineEdit
	var chooseLogo, clearLogo *walk.PushButton
	var preview *walk.ImageView
	var info *walk.TextLabel
	var bitmap *walk.Bitmap
	var result *QRCodeResult
	var structure *Structure
	var accepted QRCodeOptions
	closed := false
	modeIndex, styleIndex := 0, 0
	if q.BuildMode == "3d" {
		modeIndex = 1
	}
	for i, value := range qrStyles {
		if value == q.Style {
			styleIndex = i
		}
	}
	read := func() QRCodeOptions {
		value := q
		value.Text = text.Text()
		value.MapsX, value.MapsY = int(mapsX.Value()), int(mapsY.Value())
		value.BuildMode = "2d"
		if mode.CurrentIndex() == 1 {
			value.BuildMode = "3d"
		}
		value.Style = qrStyles[style.CurrentIndex()]
		value.ForegroundBlock, value.BackgroundBlock, value.FinderBlock = qrBlockName(foreground.CurrentIndex()), qrBlockName(background.CurrentIndex()), qrBlockName(finder.CurrentIndex())
		value.LogoPath, value.LogoPercent = strings.TrimSpace(logo.Text()), int(logoSize.Value())
		return value
	}
	lock := func(busy bool) {
		for _, control := range []walk.Widget{text, mapsX, mapsY, mode, style, foreground, background, finder, logo, logoSize, chooseLogo, clearLogo, update, create} {
			control.SetEnabled(!busy)
		}
	}
	generate := func(apply bool) {
		if a.busy {
			return
		}
		value, build := read(), a.buildOptions()
		lock(true)
		info.SetText("Генерация и проверка QR-кода…")
		var generated *QRCodeResult
		var generatedStructure *Structure
		var generationErr error
		a.startWork("Генерация QR-кода…", func() error {
			generated, generationErr = GenerateQRCode(value)
			if generationErr == nil {
				generatedStructure, generationErr = BuildStructure(generated.Preview, build)
			}
			return nil
		}, func() {
			if closed {
				return
			}
			lock(false)
			if generationErr != nil {
				info.SetText(generationErr.Error())
				a.status.SetText(generationErr.Error())
				return
			}
			next, err := walk.NewBitmapFromImageForDPI(generatedStructure.MapImage, preview.DPI())
			if err != nil {
				info.SetText(err.Error())
				return
			}
			if err := preview.SetImage(next); err != nil {
				next.Dispose()
				info.SetText(err.Error())
				return
			}
			if bitmap != nil {
				bitmap.Dispose()
			}
			bitmap = next
			info.SetText(fmt.Sprintf("QR-код читается · %d × %d элементов · %d блока на элемент\r\n%d × %d карт · %s", generated.Modules, generated.Modules, generated.ModulePixels, value.MapsX, value.MapsY, buildModeLabel(value.BuildMode)))
			a.status.SetText("QR-превью готово и проверено.")
			if apply {
				accepted, result, structure = value, generated, generatedStructure
				dlg.Accept()
			}
		})
	}
	definition := d.Dialog{AssignTo: &dlg, Title: "Генератор QR-кода — мап-арт", Size: d.Size{1050, 750}, MinSize: d.Size{850, 650}, Layout: d.VBox{}, CancelButton: &cancel, Children: []d.Widget{
		d.HSplitter{StretchFactor: 1, Children: []d.Widget{
			d.Composite{MinSize: d.Size{480, 0}, Layout: d.VBox{}, Children: []d.Widget{
				d.Label{Text: "Ссылка или текст"},
				d.TextEdit{AssignTo: &text, Text: q.Text, MinSize: d.Size{0, 70}, MaxSize: d.Size{0, 90}, VScroll: true},
				d.Composite{Layout: d.HBox{MarginsZero: true}, Children: []d.Widget{
					d.Label{Text: "Карт X × Y"},
					d.NumberEdit{AssignTo: &mapsX, Value: float64(q.MapsX), MinValue: 1, MaxValue: 256, Decimals: 0, SpinButtonsVisible: true},
					d.NumberEdit{AssignTo: &mapsY, Value: float64(q.MapsY), MinValue: 1, MaxValue: 256, Decimals: 0, SpinButtonsVisible: true},
				}},
				d.Label{Text: "Постройка"}, d.ComboBox{AssignTo: &mode, Model: qrModeLabels, CurrentIndex: modeIndex},
				d.Label{Text: "Форма элементов"}, d.ComboBox{AssignTo: &style, Model: qrStyleLabels, CurrentIndex: styleIndex},
				d.Label{Text: "Цвет QR"}, d.ComboBox{AssignTo: &foreground, Model: qrBlockLabels(), CurrentIndex: qrBlockIndex(q.ForegroundBlock)},
				d.Label{Text: "Цвет фона"}, d.ComboBox{AssignTo: &background, Model: qrBlockLabels(), CurrentIndex: qrBlockIndex(q.BackgroundBlock)},
				d.Label{Text: "Цвет углов QR"}, d.ComboBox{AssignTo: &finder, Model: qrBlockLabels(), CurrentIndex: qrBlockIndex(q.FinderBlock)},
				d.Label{Text: "Логотип в центре (необязательно)"},
				d.Composite{Layout: d.HBox{MarginsZero: true}, Children: []d.Widget{
					d.LineEdit{AssignTo: &logo, Text: q.LogoPath, CueBanner: "Картинка для центра QR"},
					d.PushButton{AssignTo: &chooseLogo, Text: "Выбрать…", OnClicked: func() {
						fd := walk.FileDialog{Title: "Логотип в центре QR", Filter: "Изображения|*.png;*.jpg;*.jpeg;*.webp;*.gif;*.bmp;*.tif;*.tiff"}
						ok, err := fd.ShowOpen(dlg)
						if err != nil {
							info.SetText(err.Error())
						} else if ok {
							logo.SetText(fd.FilePath)
						}
					}},
					d.PushButton{AssignTo: &clearLogo, Text: "Убрать", OnClicked: func() { logo.SetText("") }},
				}},
				d.Composite{Layout: d.HBox{MarginsZero: true}, Children: []d.Widget{
					d.Label{Text: "Размер логотипа, %"}, d.NumberEdit{AssignTo: &logoSize, Value: float64(q.LogoPercent), MinValue: 5, MaxValue: 20, Decimals: 0, SpinButtonsVisible: true},
				}},
				d.TextLabel{Text: "Цвета выбираются из блоков Minecraft. Вокруг кода автоматически остаётся свободное поле. После создания доступны кисти и экспорт схемы."},
				d.VSpacer{},
			}},
			d.Composite{Layout: d.VBox{}, StretchFactor: 1, Children: []d.Widget{
				d.ImageView{AssignTo: &preview, Mode: d.ImageViewModeZoom, MinSize: d.Size{300, 300}, StretchFactor: 1, Background: d.SolidColorBrush{Color: walk.RGB(235, 238, 242)}},
				d.TextLabel{AssignTo: &info, Text: "Вставьте ссылку и нажмите «Обновить превью» или «Создать мап-арт». Цвета показаны как на карте Minecraft."},
			}},
		}},
		d.Composite{Layout: d.HBox{}, Children: []d.Widget{
			d.HSpacer{},
			d.PushButton{AssignTo: &update, Text: "Обновить превью", OnClicked: func() { generate(false) }},
			d.PushButton{AssignTo: &create, Text: "Создать мап-арт", OnClicked: func() { generate(true) }},
			d.PushButton{AssignTo: &cancel, Text: "Закрыть", OnClicked: func() { dlg.Cancel() }},
		}},
	}}
	if err := definition.Create(a.window); err != nil {
		a.showError(err)
		return
	}
	defer dlg.Dispose()
	defer func() {
		if bitmap != nil {
			bitmap.Dispose()
		}
	}()
	changed := func() {
		info.SetText("Оформление изменено. Обновите превью или создайте мап-арт.")
	}
	text.TextChanged().Attach(changed)
	logo.TextChanged().Attach(changed)
	for _, field := range []*walk.NumberEdit{mapsX, mapsY, logoSize} {
		field.ValueChanged().Attach(changed)
	}
	for _, field := range []*walk.ComboBox{mode, style, foreground, background, finder} {
		field.CurrentIndexChanged().Attach(changed)
	}
	command := dlg.Run()
	closed = true
	if command == walk.DlgCmdOK && result != nil {
		a.acceptQRCode(accepted, result, structure)
	}
}
