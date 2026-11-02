//go:build !windows && gui

package main

import (
	"testing"

	"fyne.io/fyne/v2"
)

func TestStudioCatalogRecyclesRowsWithoutMovingSelections(t *testing.T) {
	g := newStudioTest(t)
	p := g.blockPicker
	p.choose("", true, false)
	row := p.list.CreateItem().(*studioBlockRow)
	p.list.UpdateItem(0, row)
	row.check.SetChecked(true)
	if !p.model.selected[0] || p.model.count != 1 {
		t.Fatal("first checkbox did not select its block")
	}
	p.list.UpdateItem(paletteBlockCount-1, row)
	if p.model.count != 1 || !p.model.selected[0] || row.check.Checked {
		t.Fatal("recycling a row changed the previous block")
	}
	row.check.SetChecked(true)
	if !p.model.selected[paletteBlockCount-1] || p.model.count != 2 {
		t.Fatal("recycled checkbox changed the wrong block")
	}
	p.list.UpdateItem(0, row)
	if !row.check.Checked || p.model.count != 2 {
		t.Fatal("scrolling back lost the selection")
	}
	p.search.SetText("pale_oak")
	p.list.UpdateItem(0, row)
	id := p.model.visible[0]
	row.check.SetChecked(true)
	if !p.model.selected[id] || p.model.count != 3 {
		t.Fatal("filtered checkbox retained its old binding")
	}
	p.list.Select(0)
	if p.model.selected[id] {
		t.Fatal("clicking the row did not toggle its checkbox")
	}
	p.list.Select(0)
	if !p.model.selected[id] {
		t.Fatal("the same row could not be toggled twice")
	}
	opts, err := g.readOptions()
	if err != nil {
		t.Fatal(err)
	}
	ids, err := selectedBlockIDs(opts.SelectedBlocks)
	if err != nil || len(ids) != 3 || !ids[0] || !ids[id] || !ids[paletteBlockCount-1] {
		t.Fatal("profile lost hidden selections", err)
	}
}

func TestStudioCatalogOnlyBuildsVisibleRowsAndSkipsUnchangedUpdates(t *testing.T) {
	g := newStudioTest(t)
	p := g.blockPicker
	created, updated := 0, 0
	create, update := p.list.CreateItem, p.list.UpdateItem
	p.list.CreateItem = func() fyne.CanvasObject { created++; return create() }
	p.list.UpdateItem = func(i int, o fyne.CanvasObject) { updated++; update(i, o) }
	p.list.Refresh()
	created, updated = 0, 0
	for i := 0; i < 8; i++ {
		g.changed()
	}
	if updated != 0 {
		t.Fatal("unrelated settings redrew the catalogue", updated)
	}
	p.list.ScrollToBottom()
	p.list.ScrollToTop()
	if created > 30 {
		t.Fatal("list created rows for the whole catalogue", created)
	}
	p.search.SetText("pale_oak")
	updated = 0
	p.search.SetText(" PALE_OAK ")
	if updated != 0 {
		t.Fatal("equivalent search rebuilt rows", updated)
	}
	p.group.SetSelected("Древесина")
	for _, id := range p.model.visible {
		if blockGroup(id) != "Древесина" {
			t.Fatal("group filter ignored")
		}
	}
	if len(g.controls) > 150 {
		t.Fatal("offscreen checkboxes entered the global UI update loop", len(g.controls))
	}
}

func TestStudioCatalogLocksSelectionDuringConversionAndAutoMode(t *testing.T) {
	g := newStudioTest(t)
	p := g.blockPicker
	p.choose("Шерсть", true, true)
	row := p.list.CreateItem().(*studioBlockRow)
	g.busy = true
	g.updateButtons()
	p.list.UpdateItem(0, row)
	before := p.model.selected
	p.toggle(0)
	p.choose("", true, false)
	if !row.check.Disabled() || p.model.selected != before {
		t.Fatal("conversion did not lock catalogue selection")
	}
	g.busy = false
	g.updateButtons()
	p.list.UpdateItem(0, row)
	if row.check.Disabled() {
		t.Fatal("conversion left selection locked")
	}
	p.auto.SetChecked(true)
	g.opts.AllBlocks = true
	g.changed()
	p.list.UpdateItem(0, row)
	if !row.check.Disabled() || p.model.count != paletteBlockCount {
		t.Fatal("automatic full palette not synchronized")
	}
	p.toggle(0)
	if p.model.count != paletteBlockCount {
		t.Fatal("read-only list changed automatic selection")
	}
	p.auto.SetChecked(false)
	p.toggle(0)
	if p.model.count != paletteBlockCount-1 || p.model.selected[0] {
		t.Fatal("leaving automatic mode lost or locked selection")
	}
}
