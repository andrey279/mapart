package main

import (
	"fmt"
	"image/color"
	"math"
	"strings"
	"sync"
)

func palettePixel(p MapColor) color.RGBA {
	return color.RGBA{uint8(math.Round(p.R)), uint8(math.Round(p.G)), uint8(math.Round(p.B)), 255}
}

var dyeLabels = [...]string{"Белая", "Оранжевая", "Пурпурная", "Голубая", "Жёлтая", "Лаймовая", "Розовая", "Серая", "Светло-серая", "Бирюзовая", "Фиолетовая", "Синяя", "Коричневая", "Зелёная", "Красная", "Чёрная"}

// Solid blocks with additional map colours from Java 1.20.1 MapColor.
// IDs 0..31 retain their existing wool/terracotta meanings.
var naturalBlocks = [...]struct {
	name, label string
	rgb         int
}{
	{"clay", "Глина", 0xa4a8b8},
	{"prismarine_bricks", "Призмариновые кирпичи", 0x5cdbd5},
	{"packed_ice", "Плотный лёд", 0xa0a0ff},
	{"sandstone", "Песчаник", 0xf7e9a3},
	{"stone", "Камень", 0x707070},
	{"oak_planks", "Дубовые доски", 0x8f7748},
}

// The first 38 IDs stay stable for existing profiles and pixel editors.
const legacyBlockCount = 32 + len(naturalBlocks)
const paletteBlockCount = legacyBlockCount + len(extraBlocks)

type catalogBlock struct {
	selection, name, label, group string
	rgb                           int
	properties                    map[string]string
	minDataVersion                int
	requiresSupport               bool
}

func blockSelectionName(id int) string {
	if id >= legacyBlockCount && id < paletteBlockCount {
		return "minecraft:" + extraBlocks[id-legacyBlockCount].selection
	}
	name, _, _ := paletteBlock(id)
	return name
}

func blockProperties(id int) map[string]string {
	if id >= legacyBlockCount && id < paletteBlockCount {
		return extraBlocks[id-legacyBlockCount].properties
	}
	return nil
}

func blockNeedsSupport(id int) bool {
	return id >= legacyBlockCount && id < paletteBlockCount && extraBlocks[id-legacyBlockCount].requiresSupport
}

func blockGroup(id int) string {
	switch {
	case id < 16:
		return "Шерсть"
	case id < 32:
		return "Терракота"
	case id < legacyBlockCount:
		return "Дополнительные материалы"
	default:
		return extraBlocks[id-legacyBlockCount].group
	}
}

var blockGroups = func() []string {
	var groups []string
	for _, group := range []string{"Шерсть", "Терракота", "Дополнительные материалы", "Древесина", "Бамбук", "Грибы и Незер", "Камень и прочее", "Руды и металлы", "Природа"} {
		for id := 0; id < paletteBlockCount; id++ {
			if blockGroup(id) == group {
				groups = append(groups, group)
				break
			}
		}
	}
	return groups
}()

type blockCatalogIndex struct {
	ids    map[string]int
	search [paletteBlockCount]string
}

var catalogIndex = sync.OnceValue(func() *blockCatalogIndex {
	index := &blockCatalogIndex{ids: make(map[string]int, paletteBlockCount)}
	for id := 0; id < paletteBlockCount; id++ {
		name := blockSelectionName(id)
		index.ids[name] = id
		index.search[id] = strings.ToLower(blockLabel(id) + " " + name + " " + blockGroup(id))
	}
	for name, replacement := range removedBlockAliases {
		id, ok := index.ids["minecraft:"+replacement]
		if !ok {
			panic("removed material has no retained equivalent: " + name)
		}
		index.ids["minecraft:"+name] = id
	}
	return index
})

func usesRemovedBlocks(selection string) bool {
	for _, name := range strings.Split(selection, ",") {
		if _, ok := removedBlockAliases[strings.TrimPrefix(strings.TrimSpace(name), "minecraft:")]; ok {
			return true
		}
	}
	return false
}

func blockMatchesSearch(id int, query string) bool {
	return blockMatchesWords(id, strings.Fields(strings.ToLower(query)))
}

func blockMatchesWords(id int, words []string) bool {
	value := catalogIndex().search[id]
	for _, word := range words {
		if !strings.Contains(value, word) {
			return false
		}
	}
	return true
}

func blockLabel(id int) string {
	if id < 0 || id >= paletteBlockCount {
		return "Неизвестный блок"
	}
	if id >= legacyBlockCount {
		return extraBlocks[id-legacyBlockCount].label
	}
	if id >= 32 {
		return naturalBlocks[id-32].label
	}
	kind := "шерсть"
	if id >= 16 {
		kind = "терракота"
	}
	return dyeLabels[id%16] + " " + kind
}

// Empty means the legacy automatic selection (wool and optional terracotta).
// An explicit list may contain even a single block for single-colour builds.
func selectedBlockIDs(value string) (map[int]bool, error) {
	if value == "" {
		return nil, nil
	}
	ids := make(map[int]bool)
	for _, entry := range strings.Split(value, ",") {
		name := strings.TrimSpace(entry)
		if name == "" {
			return nil, fmt.Errorf("выберите хотя бы один блок; список не должен содержать пустых элементов")
		}
		if !strings.HasPrefix(name, "minecraft:") {
			name = "minecraft:" + name
		}
		id, found := catalogIndex().ids[name]
		if !found {
			return nil, fmt.Errorf("неизвестный блок %q: выберите материал из вкладки «Блоки»", entry)
		}
		ids[id] = true
	}
	return ids, nil
}

func activeBlockIDs(opts RenderOptions) map[int]bool {
	ids, _ := selectedBlockIDs(opts.SelectedBlocks)
	if ids == nil {
		ids = make(map[int]bool)
		for id := 0; id < paletteBlockCount; id++ {
			ids[id] = opts.AllBlocks || id < 16 || (id < 32 && opts.Terracotta) || (id >= 32 && id < legacyBlockCount && opts.NaturalBlocks)
		}
	}
	return ids
}

func blockSelection(ids map[int]bool) string {
	var names []string
	for id := 0; id < paletteBlockCount; id++ {
		if ids[id] {
			names = append(names, blockSelectionName(id))
		}
	}
	return strings.Join(names, ",")
}

func buildModeLabel(mode string) string {
	if mode == "2d" {
		return "2D — плоская постройка"
	}
	return "3D — рельеф для оттенков"
}

// Java MapColor TERRACOTTA_WHITE .. TERRACOTTA_BLACK, IDs 36..51.
// The order matches woolNames (Minecraft's "lime" is COLOR_LIGHT_GREEN).
// Only the three shades achievable with height changes are included.
var terracottaBaseColors = [...]int{
	13742497, 10441252, 9787244, 7367818,
	12223780, 6780213, 10505550, 3746083,
	8874850, 5725276, 8014168, 4996700,
	4993571, 5001770, 9321518, 2430480,
}

// WoolID is the historical palette block index: wool, terracotta, then natural.
func paletteBlock(id int) (name string, base [3]float64, ok bool) {
	if id >= legacyBlockCount && id < paletteBlockCount {
		block := extraBlocks[id-legacyBlockCount]
		return "minecraft:" + block.name, [3]float64{float64(block.rgb >> 16 & 255), float64(block.rgb >> 8 & 255), float64(block.rgb & 255)}, true
	}
	if id >= 32 && id < legacyBlockCount {
		block := naturalBlocks[id-32]
		return "minecraft:" + block.name, [3]float64{float64(block.rgb >> 16 & 255), float64(block.rgb >> 8 & 255), float64(block.rgb & 255)}, true
	}
	if id >= 0 && id < len(baseColors) {
		return "minecraft:" + woolNames[id] + "_wool", baseColors[id], true
	}
	id -= len(baseColors)
	if id >= 0 && id < len(terracottaBaseColors) {
		c := terracottaBaseColors[id]
		return "minecraft:" + woolNames[id] + "_terracotta", [3]float64{float64(c >> 16 & 255), float64(c >> 8 & 255), float64(c & 255)}, true
	}
	return "", base, false
}

func extraPalette() []MapColor {
	colors := make([]MapColor, 0, len(extraBlocks)*3)
	for i := range extraBlocks {
		id := legacyBlockCount + i
		_, base, _ := paletteBlock(id)
		for shade, modifier := range []int{180, 220, 255} {
			r, g, b := float64(int(base[0])*modifier/255), float64(int(base[1])*modifier/255), float64(int(base[2])*modifier/255)
			l, a, lb := rgbToLab(r, g, b)
			colors = append(colors, MapColor{r, g, b, l, a, lb, []string{"Dark", "Normal", "Light"}[shade], id})
		}
	}
	return colors
}

func naturalPalette() []MapColor {
	colors := make([]MapColor, 0, len(naturalBlocks)*3)
	for i := range naturalBlocks {
		id := 32 + i
		_, base, _ := paletteBlock(id)
		for shade, modifier := range []int{180, 220, 255} {
			r, g, b := float64(int(base[0])*modifier/255), float64(int(base[1])*modifier/255), float64(int(base[2])*modifier/255)
			l, a, lb := rgbToLab(r, g, b)
			colors = append(colors, MapColor{r, g, b, l, a, lb, []string{"Dark", "Normal", "Light"}[shade], id})
		}
	}
	return colors
}

func terracottaPalette() []MapColor {
	colors := make([]MapColor, 0, 48)
	for id := range terracottaBaseColors {
		_, base, _ := paletteBlock(len(baseColors) + id)
		for i, modifier := range []int{180, 220, 255} {
			// Exact Java integer colours. Keep legacy wool rounding unchanged.
			r := float64(int(base[0]) * modifier / 255)
			g := float64(int(base[1]) * modifier / 255)
			b := float64(int(base[2]) * modifier / 255)
			l, a, lb := rgbToLab(r, g, b)
			colors = append(colors, MapColor{r, g, b, l, a, lb, []string{"Dark", "Normal", "Light"}[i], len(baseColors) + id})
		}
	}
	return colors
}
