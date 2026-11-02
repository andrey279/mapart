package main

import "strings"

const allBlockGroupsLabel = "Все группы"

// Selection belongs to the catalogue, never to recycled row widgets. Both
// desktop interfaces draw only the visible part of this small, stable model.
type blockPickerModel struct {
	selected     [paletteBlockCount]bool
	visible      []int
	count        int
	revision     uint64
	query, group string
}

func newBlockPickerModel() *blockPickerModel {
	m := &blockPickerModel{visible: make([]int, paletteBlockCount)}
	for id := range m.visible {
		m.visible[id] = id
	}
	return m
}

func (m *blockPickerModel) filter(query, group string) bool {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == m.query && group == m.group {
		return false
	}
	m.query, m.group = query, group
	words := strings.Fields(query)
	m.visible = m.visible[:0]
	for id := 0; id < paletteBlockCount; id++ {
		if (group == "" || blockGroup(id) == group) && blockMatchesWords(id, words) {
			m.visible = append(m.visible, id)
		}
	}
	return true
}

func (m *blockPickerModel) setChecked(id int, checked bool) bool {
	if id < 0 || id >= paletteBlockCount || m.selected[id] == checked {
		return false
	}
	m.selected[id] = checked
	if checked {
		m.count++
	} else {
		m.count--
	}
	m.revision++
	return true
}

func (m *blockPickerModel) load(opts RenderOptions) {
	ids := activeBlockIDs(opts)
	for id := range m.selected {
		m.setChecked(id, ids[id])
	}
}

func (m *blockPickerModel) selectGroup(group string, replace, checked bool) {
	for id := range m.selected {
		if group == "" || blockGroup(id) == group {
			m.setChecked(id, checked)
		} else if replace {
			m.setChecked(id, false)
		}
	}
}

func (m *blockPickerModel) selection() string {
	ids := make(map[int]bool, m.count)
	for id, checked := range m.selected {
		if checked {
			ids[id] = true
		}
	}
	return blockSelection(ids)
}

type blockAutoPalette struct {
	terra, natural, all bool
}

func autoBlockPalette(opts RenderOptions) blockAutoPalette {
	return blockAutoPalette{opts.Terracotta, opts.NaturalBlocks, opts.AllBlocks}
}

func (p blockAutoPalette) options() RenderOptions {
	return RenderOptions{Terracotta: p.terra, NaturalBlocks: p.natural, AllBlocks: p.all}
}
