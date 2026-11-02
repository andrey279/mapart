//go:build windows

package main

import (
	"fmt"
	"strings"

	"github.com/lxn/walk"
	d "github.com/lxn/walk/declarative"
)

type nativeBlockTable struct {
	walk.TableModelBase
	model     *blockPickerModel
	readOnly  bool
	onChanged func()
}

func (m *nativeBlockTable) RowCount() int { return len(m.model.visible) }
func (m *nativeBlockTable) Value(row, col int) interface{} {
	id := m.model.visible[row]
	if col == 0 {
		return blockLabel(id)
	}
	return strings.TrimPrefix(blockSelectionName(id), "minecraft:")
}
func (m *nativeBlockTable) Checked(row int) bool { return m.model.selected[m.model.visible[row]] }
func (m *nativeBlockTable) SetChecked(row int, checked bool) error {
	if !m.readOnly && m.model.setChecked(m.model.visible[row], checked) {
		m.PublishRowChanged(row)
		m.onChanged()
	}
	return nil
}

func (a *nativeApp) blockPage() d.TabPage {
	var auto *walk.CheckBox
	var info *walk.Label
	var search *walk.LineEdit
	var group *walk.ComboBox
	var table *walk.TableView
	model := newBlockPickerModel()
	rows := &nativeBlockTable{model: model, onChanged: a.changed}
	updating, synced, lastAuto, lastReadOnly := false, false, false, false
	var lastPalette blockAutoPalette
	var lastRevision uint64
	updateInfo := func() {
		if info != nil {
			info.SetText(fmt.Sprintf("Выбрано: %d из %d · Найдено: %d", model.count, paletteBlockCount, len(model.visible)))
		}
	}
	syncState := func() {
		if auto == nil || table == nil {
			return
		}
		palette := autoBlockPalette(a.options)
		if auto.Checked() && (!synced || !lastAuto || lastPalette != palette) {
			model.load(palette.options())
		}
		readOnly := auto.Checked() || a.busy
		rows.readOnly = readOnly
		if !synced || lastReadOnly != readOnly {
			table.SetEnabled(!readOnly)
		}
		if (!synced || lastRevision != model.revision) && len(model.visible) > 0 {
			rows.PublishRowsChanged(0, len(model.visible)-1)
		}
		lastPalette, lastAuto, lastReadOnly, lastRevision, synced = palette, auto.Checked(), readOnly, model.revision, true
		updateInfo()
	}
	a.syncBlockControls = syncState
	choose := func(name string, replace, checked bool) {
		if a.busy {
			return
		}
		updating = true
		auto.SetChecked(false)
		model.selectGroup(name, replace, checked)
		updating = false
		a.changed()
	}
	currentGroup := func() string {
		if group == nil || group.Text() == allBlockGroupsLabel {
			return ""
		}
		return group.Text()
	}
	filter := func() {
		if search == nil || group == nil || table == nil {
			return
		}
		if model.filter(search.Text(), currentGroup()) {
			rows.PublishRowsReset()
			table.SetCurrentIndex(-1)
			updateInfo()
		}
	}
	a.readFields = append(a.readFields, func() {
		if auto.Checked() {
			a.options.SelectedBlocks = ""
			return
		}
		a.options.SelectedBlocks = model.selection()
		if a.options.SelectedBlocks == "" {
			a.options.SelectedBlocks = ","
		}
	})
	a.writeFields = append(a.writeFields, func() {
		updating = true
		model.load(a.options)
		auto.SetChecked(a.options.SelectedBlocks == "")
		updating = false
		synced = false
		syncState()
	})
	return d.TabPage{Title: "Блоки", Layout: d.VBox{MarginsZero: true}, Children: []d.Widget{
		d.CheckBox{AssignTo: &auto, Text: "Автоматическая палитра", OnCheckedChanged: func() {
			if !updating {
				a.changed()
			}
		}},
		d.Composite{Layout: d.HBox{MarginsZero: true}, Children: []d.Widget{
			d.PushButton{Text: "Шерсть", OnClicked: func() { choose("Шерсть", true, true) }},
			d.PushButton{Text: "Терракота", OnClicked: func() { choose("Терракота", true, true) }},
			d.PushButton{Text: "Все", OnClicked: func() { choose("", true, true) }},
			d.PushButton{Text: "Снять", OnClicked: func() { choose("", true, false) }},
		}},
		d.LineEdit{AssignTo: &search, CueBanner: "Поиск: шерсть, древесина, гриб, deepslate…", OnTextChanged: filter},
		d.ComboBox{AssignTo: &group, Model: append([]string{allBlockGroupsLabel}, blockGroups...), CurrentIndex: 0, OnCurrentIndexChanged: filter},
		d.Composite{Layout: d.HBox{MarginsZero: true}, Children: []d.Widget{
			d.PushButton{Text: "Добавить группу", OnClicked: func() { choose(currentGroup(), false, true) }},
			d.PushButton{Text: "Убрать группу", OnClicked: func() { choose(currentGroup(), false, false) }},
		}},
		d.Label{AssignTo: &info},
		d.TableView{AssignTo: &table, Model: rows, CheckBoxes: true, LastColumnStretched: true, NotSortableByHeaderClick: true, StretchFactor: 1,
			Columns: []d.TableViewColumn{{Title: "Материал", Width: 235}, {Title: "Minecraft ID", Width: 240}}},
	}}
}
