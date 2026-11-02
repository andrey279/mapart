//go:build !windows && gui

package main

import (
	"fmt"
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

type studioBlockRow struct {
	widget.BaseWidget
	check         *widget.Check
	title, detail *widget.Label
	swatch        *canvas.Rectangle
	content       fyne.CanvasObject
}

func newStudioBlockRow() *studioBlockRow {
	r := &studioBlockRow{check: widget.NewCheck("", nil), title: widget.NewLabel(""), detail: widget.NewLabel("")}
	r.title.Truncation = fyne.TextTruncateEllipsis
	r.detail.Truncation = fyne.TextTruncateEllipsis
	r.swatch = canvas.NewRectangle(color.Transparent)
	r.swatch.SetMinSize(fyne.NewSize(18, 18))
	minimum := canvas.NewRectangle(color.Transparent)
	minimum.SetMinSize(fyne.NewSize(200, 56))
	left := container.NewHBox(container.NewCenter(r.swatch), r.check)
	r.content = container.NewStack(minimum, container.NewBorder(nil, nil, left, nil, container.NewVBox(r.title, r.detail)))
	r.ExtendBaseWidget(r)
	return r
}

func (r *studioBlockRow) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(r.content)
}

type studioBlockPicker struct {
	g                                        *FyneGUI
	model                                    *blockPickerModel
	list                                     *widget.List
	auto                                     *widget.Check
	search                                   *widget.Entry
	group                                    *widget.Select
	info                                     *widget.Label
	updating, synced, lastAuto, lastReadOnly bool
	lastPalette                              blockAutoPalette
	lastRevision                             uint64
}

func (p *studioBlockPicker) readOnly() bool { return p.auto.Checked || p.g.busy }

func (p *studioBlockPicker) updateInfo() {
	p.info.SetText(fmt.Sprintf("Выбрано: %d из %d · Найдено: %d", p.model.count, paletteBlockCount, len(p.model.visible)))
}

func (p *studioBlockPicker) sync() {
	palette := autoBlockPalette(p.g.opts)
	if p.auto.Checked && (!p.synced || !p.lastAuto || palette != p.lastPalette) {
		p.model.load(palette.options())
	}
	readOnly := p.readOnly()
	if !p.synced || p.lastRevision != p.model.revision || p.lastReadOnly != readOnly {
		p.list.Refresh()
	}
	p.lastPalette, p.lastAuto, p.lastReadOnly, p.lastRevision, p.synced = palette, p.auto.Checked, readOnly, p.model.revision, true
	p.updateInfo()
}

func (p *studioBlockPicker) choose(group string, replace, checked bool) {
	if p.g.busy {
		return
	}
	p.updating = true
	p.auto.SetChecked(false)
	p.model.selectGroup(group, replace, checked)
	p.updating = false
	p.g.changed()
}

func (p *studioBlockPicker) currentGroup() string {
	if p.group.Selected == allBlockGroupsLabel {
		return ""
	}
	return p.group.Selected
}

func (p *studioBlockPicker) filter() {
	if !p.model.filter(p.search.Text, p.currentGroup()) {
		return
	}
	p.list.ScrollToTop()
	p.list.Refresh()
	p.updateInfo()
}

func (p *studioBlockPicker) toggle(row int) {
	if p.readOnly() || row < 0 || row >= len(p.model.visible) {
		return
	}
	id := p.model.visible[row]
	if p.model.setChecked(id, !p.model.selected[id]) {
		p.g.changed()
	}
}

func (g *FyneGUI) blockPage() *container.TabItem {
	p := &studioBlockPicker{g: g, model: newBlockPickerModel(), auto: widget.NewCheck("Автоматическая палитра", nil), info: wrapped("")}
	g.blockPicker = p
	p.list = widget.NewList(func() int { return len(p.model.visible) }, func() fyne.CanvasObject { return newStudioBlockRow() }, func(row widget.ListItemID, object fyne.CanvasObject) {
		if row < 0 || row >= len(p.model.visible) {
			return
		}
		id := p.model.visible[row]
		r := object.(*studioBlockRow)
		// Reset a recycled checkbox without writing its previous row's state.
		r.check.OnChanged = nil
		r.check.SetChecked(p.model.selected[id])
		if p.readOnly() {
			r.check.Disable()
		} else {
			r.check.Enable()
		}
		r.check.OnChanged = func(checked bool) {
			if !p.readOnly() && p.model.setChecked(id, checked) {
				g.changed()
			}
		}
		r.title.SetText(blockLabel(id))
		r.detail.SetText(blockGroup(id) + " · " + strings.TrimPrefix(blockSelectionName(id), "minecraft:"))
		_, rgb, _ := paletteBlock(id)
		r.swatch.FillColor = color.NRGBA{uint8(rgb[0]), uint8(rgb[1]), uint8(rgb[2]), 255}
		r.swatch.Refresh()
	})
	p.list.OnSelected = func(row widget.ListItemID) { p.list.UnselectAll(); p.toggle(row) }
	p.auto.OnChanged = func(bool) {
		if !p.updating {
			g.changed()
		}
	}
	g.track(p.auto)
	g.syncBlockControls = p.sync
	g.readFields = append(g.readFields, func() error {
		if p.auto.Checked {
			g.opts.SelectedBlocks = ""
			return nil
		}
		if p.model.count == 0 {
			return fmt.Errorf("выберите хотя бы один блок во вкладке «Блоки»")
		}
		g.opts.SelectedBlocks = p.model.selection()
		return nil
	})
	g.writeFields = append(g.writeFields, func() {
		p.updating = true
		p.model.load(g.opts)
		p.auto.SetChecked(g.opts.SelectedBlocks == "")
		p.updating = false
		p.synced = false
		p.sync()
	})
	p.search = widget.NewEntry()
	p.search.SetPlaceHolder("Поиск: шерсть, древесина, гриб, deepslate…")
	p.group = widget.NewSelect(append([]string{allBlockGroupsLabel}, blockGroups...), func(string) { p.filter() })
	p.group.Selected = allBlockGroupsLabel
	p.search.OnChanged = func(string) { p.filter() }
	g.track(p.search)
	g.track(p.group)
	quick := container.NewGridWithColumns(2,
		g.action("Шерсть", nil, func() { p.choose("Шерсть", true, true) }),
		g.action("Терракота", nil, func() { p.choose("Терракота", true, true) }),
		g.action("Все блоки", nil, func() { p.choose("", true, true) }),
		g.action("Снять выбор", nil, func() { p.choose("", true, false) }))
	groupActions := container.NewGridWithColumns(2,
		g.action("Добавить группу", nil, func() { p.choose(p.currentGroup(), false, true) }),
		g.action("Убрать группу", nil, func() { p.choose(p.currentGroup(), false, false) }))
	header := container.NewVBox(p.auto, quick, p.search, p.group, groupActions, p.info)
	return container.NewTabItem("Блоки", container.NewBorder(header, nil, nil, nil, p.list))
}
