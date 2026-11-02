package main

import (
	"reflect"
	"testing"
)

func TestBlockPickerKeepsSelectionAcrossFiltersAndGroups(t *testing.T) {
	m := newBlockPickerModel()
	m.load(DefaultRenderOptions())
	if m.count != 16 {
		t.Fatal("default wool selection", m.count)
	}
	selected := m.selected
	if !m.filter("  PALE_OAK  ", "Древесина") || len(m.visible) == 0 {
		t.Fatal("group search has no results")
	}
	for _, id := range m.visible {
		if blockGroup(id) != "Древесина" || !blockMatchesSearch(id, "pale_oak") {
			t.Fatal("unmatched row")
		}
	}
	if m.selected != selected {
		t.Fatal("search altered hidden selections")
	}
	if m.filter("pale_oak", "Древесина") {
		t.Fatal("same filter needlessly invalidated rows")
	}
	id := m.visible[len(m.visible)-1]
	m.setChecked(id, true)
	m.filter("no_such_block", "Природа")
	if len(m.visible) != 0 || !m.selected[id] || m.count != 17 {
		t.Fatal("empty search lost selection")
	}
	m.selectGroup("Терракота", true, true)
	if m.count != 16 {
		t.Fatal("terracotta group incomplete", m.count)
	}
	m.selectGroup("Грибы и Незер", false, true)
	for i, v := range m.selected {
		if v != (blockGroup(i) == "Терракота" || blockGroup(i) == "Грибы и Незер") {
			t.Fatal("group addition replaced another group")
		}
	}
	m.selectGroup("Терракота", false, false)
	ids, err := selectedBlockIDs(m.selection())
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < paletteBlockCount; i++ {
		if ids[i] != (blockGroup(i) == "Грибы и Незер") {
			t.Fatal("hidden group removal", i)
		}
	}
	m.filter("", "")
	m.selectGroup("", true, true)
	if len(m.visible) != paletteBlockCount || m.count != paletteBlockCount {
		t.Fatal("catalogue truncated")
	}
	roundTrip := newBlockPickerModel()
	roundTrip.load(RenderOptions{SelectedBlocks: m.selection()})
	if !reflect.DeepEqual(roundTrip.selected, m.selected) || roundTrip.count != m.count {
		t.Fatal("full selection does not round trip")
	}
}
