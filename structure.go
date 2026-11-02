package main

import (
	"archive/zip"
	"bufio"
	"compress/gzip"
	"encoding/binary"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Palette order matches baseColors. Wool has no gravity or block state
// properties, so every surface entry can be reproduced by one stable block.
var woolNames = [...]string{"white", "orange", "magenta", "light_blue", "yellow", "lime", "pink", "gray", "light_gray", "cyan", "purple", "blue", "brown", "green", "red", "black"}

type BuildOptions struct {
	Supports    bool `json:"supports"`
	MaxHeight   int  `json:"max_height"`
	DataVersion int  `json:"data_version"`
}

func DefaultBuildOptions() BuildOptions {
	return BuildOptions{Supports: true, MaxHeight: 384, DataVersion: 5023} // Java 26.3
}

func (o BuildOptions) Validate() error {
	if o.MaxHeight < 2 || o.MaxHeight > 4096 {
		return fmt.Errorf("max-height должен быть в пределах 2..4096")
	}
	if o.DataVersion < 1519 || o.DataVersion > math.MaxInt32 {
		return fmt.Errorf("data-version должен соответствовать Java 1.13 или новее (>=1519)")
	}
	return nil
}

type StructureBlock struct{ X, Y, Z, State int }
type Material struct {
	Block     string `json:"block"`
	Count     int    `json:"count"`
	Stacks    int    `json:"stacks"`
	Remainder int    `json:"remainder"`
}
type StructureInfo struct {
	BuildMode     string     `json:"build_mode"`
	Size          [3]int     `json:"size"`
	ImageSize     [2]int     `json:"image_size"`
	ImageOffset   [3]int     `json:"image_offset"`
	DataVersion   int        `json:"data_version"`
	SurfaceBlocks int        `json:"surface_blocks"`
	NorthBlocks   int        `json:"north_blocks"`
	SupportBlocks int        `json:"support_blocks"`
	TotalBlocks   int        `json:"total_blocks"`
	Materials     []Material `json:"materials"`
	Placement     string     `json:"placement"`
}
type Structure struct {
	Info       StructureInfo
	Palette    []string
	Properties []map[string]string // NBT states, parallel to Palette.
	Blocks     []StructureBlock
	Heights    []int // Includes north row; row major X, then Z.
	MapImage   *image.RGBA
}

func structureColorLookup(result *PreviewResult) (map[color.RGBA]MapColor, error) {
	if result == nil || result.Image == nil || result.Image.Bounds().Empty() {
		return nil, fmt.Errorf("сначала создайте превью")
	}
	lookup := make(map[color.RGBA]MapColor)
	for _, p := range result.Palette {
		if _, _, ok := paletteBlock(p.WoolID); !ok || (p.Shade != "Dark" && p.Shade != "Normal" && p.Shade != "Light") {
			return nil, fmt.Errorf("экспорт NBT поддерживает встроенную палитру блоков; произвольная RGB-палитра не задаёт блоки Minecraft")
		}
		pixel := palettePixel(p)
		if _, exists := lookup[pixel]; !exists {
			lookup[pixel] = p
		}
	}
	return lookup, nil
}

// BuildStructure preserves every selected tone. A rise produces Light, equal
// height Normal, a fall Dark (scale-0 Java maps). Normal runs are equalities;
// two directed passes produce the lowest feasible nonnegative column profile.
// Falls/rises can be steeper than one block, compressing long mixed staircases.
func BuildStructure(result *PreviewResult, opts BuildOptions) (*Structure, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}
	lookup, err := structureColorLookup(result)
	if err != nil {
		return nil, err
	}
	bounds := result.Image.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if w*h > 256*128*128 {
		return nil, fmt.Errorf("схема превышает 256 карт")
	}
	top := make([]MapColor, w*h)
	mandatorySupport := false
	for z := 0; z < h; z++ {
		for x := 0; x < w; x++ {
			p, ok := lookup[result.Image.RGBAAt(bounds.Min.X+x, bounds.Min.Y+z)]
			if !ok {
				return nil, fmt.Errorf("пиксель %d,%d отсутствует в палитре блоков", x, z)
			}
			top[z*w+x] = p
			mandatorySupport = mandatorySupport || blockNeedsSupport(p.WoolID)
			if p.WoolID >= legacyBlockCount {
				block := extraBlocks[p.WoolID-legacyBlockCount]
				if opts.DataVersion < block.minDataVersion {
					return nil, fmt.Errorf("%s требует DataVersion >= %d; для полного каталога выберите 5023 (Java 26.3)", block.label, block.minDataVersion)
				}
			}
			if result.BuildMode == "2d" && p.Shade != "Normal" {
				return nil, fmt.Errorf("режим 2D допускает только обычные оттенки; преобразуйте изображение в режиме 2D")
			}
		}
	}
	s := &Structure{Heights: make([]int, w*(h+1)), MapImage: image.NewRGBA(image.Rect(0, 0, w, h))}
	maxY := 0
	for x := 0; x < w; x++ {
		for z := 1; z <= h; z++ {
			switch top[(z-1)*w+x].Shade {
			case "Light":
				s.Heights[z*w+x] = s.Heights[(z-1)*w+x] + 1
			case "Normal":
				s.Heights[z*w+x] = s.Heights[(z-1)*w+x]
			}
		}
		for z := h - 1; z >= 0; z-- {
			switch top[z*w+x].Shade {
			case "Dark":
				s.Heights[z*w+x] = max(s.Heights[z*w+x], s.Heights[(z+1)*w+x]+1)
			case "Normal":
				s.Heights[z*w+x] = max(s.Heights[z*w+x], s.Heights[(z+1)*w+x])
			}
		}
		for z := 0; z <= h; z++ {
			if mandatorySupport {
				s.Heights[z*w+x]++
			}
			maxY = max(maxY, s.Heights[z*w+x])
		}
	}
	if maxY+1 > opts.MaxHeight {
		return nil, fmt.Errorf("нужна высота %d блоков, лимит %d; сохраните карты отдельно через -nbt-tiles или кнопку ZIP по картам", maxY+1, opts.MaxHeight)
	}
	s.Info = StructureInfo{Size: [3]int{w, maxY + 1, h + 1}, ImageSize: [2]int{w, h}, ImageOffset: [3]int{0, 0, 1}, DataVersion: opts.DataVersion, SurfaceBlocks: w * h, NorthBlocks: w,
		Placement: "Верх картинки = север (-Z), справа = восток (+X). Не вращать и не зеркалить. Арт начинается на 1 блок южнее начала схемы; ряд Z=0 служебный. Для карты масштаба 0 угол арта X=-64+128*k, Z=-64+128*m; угол схемы Z=-65+128*m. Над поверхностью арта не должно быть посторонних блоков. Основание Y выбирайте так, чтобы вся схема помещалась в высоту мира."}
	s.Info.BuildMode = "3d"
	if result.BuildMode == "2d" {
		s.Info.BuildMode = "2d"
	}
	states := map[string]int{}
	counts := map[string]int{}
	add := func(x, y, z int, name string, id int) {
		key := name
		if id >= 0 {
			key = blockSelectionName(id)
		}
		state, ok := states[key]
		if !ok {
			state = len(s.Palette)
			states[key] = state
			s.Palette = append(s.Palette, name)
			s.Properties = append(s.Properties, blockProperties(id))
		}
		s.Blocks = append(s.Blocks, StructureBlock{x, y, z, state})
		counts[name]++
	}
	for z := 0; z <= h; z++ {
		for x := 0; x < w; x++ {
			y := s.Heights[z*w+x]
			if z == 0 {
				add(x, y, z, "minecraft:cobblestone", -1)
			} else {
				p := top[(z-1)*w+x]
				name, base, _ := paletteBlock(p.WoolID)
				add(x, y, z, name, p.WoolID)
				modifier := 220
				if p.Shade == "Light" {
					modifier = 255
				} else if p.Shade == "Dark" {
					modifier = 180
				}
				// Minecraft truncates integer channels; the legacy PNG rounds them.
				s.MapImage.SetRGBA(x, z-1, color.RGBA{uint8(int(base[0]) * modifier / 255), uint8(int(base[1]) * modifier / 255), uint8(int(base[2]) * modifier / 255), 255})
			}
			bottom := y
			if z > 0 && blockNeedsSupport(top[(z-1)*w+x].WoolID) {
				bottom = y - 1
			}
			if opts.Supports {
				if z > 0 {
					bottom = min(bottom, s.Heights[(z-1)*w+x])
				}
				if z < h {
					bottom = min(bottom, s.Heights[(z+1)*w+x])
				}
			}
			for supportY := bottom; supportY < y; supportY++ {
				add(x, supportY, z, "minecraft:cobblestone", -1)
				s.Info.SupportBlocks++
			}
		}
	}
	s.Info.TotalBlocks = len(s.Blocks)
	for name, count := range counts {
		s.Info.Materials = append(s.Info.Materials, Material{name, count, count / 64, count % 64})
	}
	sort.Slice(s.Info.Materials, func(i, j int) bool { return s.Info.Materials[i].Block < s.Info.Materials[j].Block })
	return s, nil
}

// Minimal, streaming big-endian Java NBT writer. No custom schematic schema:
// gzip(root compound {DataVersion,size,palette,blocks,entities}).
type nbtWriter struct {
	w   io.Writer
	err error
}

func (n *nbtWriter) raw(v any) {
	if n.err == nil {
		n.err = binary.Write(n.w, binary.BigEndian, v)
	}
}
func (n *nbtWriter) str(s string) {
	n.raw(uint16(len(s)))
	if n.err == nil {
		_, n.err = io.WriteString(n.w, s)
	}
}
func (n *nbtWriter) tag(kind byte, name string) { n.raw(kind); n.str(name) }
func (n *nbtWriter) ints(name string, v [3]int) {
	n.tag(9, name)
	n.raw(byte(3))
	n.raw(int32(3))
	for _, i := range v {
		n.raw(int32(i))
	}
}
func WriteStructureNBT(w io.Writer, s *Structure) error {
	if s == nil {
		return fmt.Errorf("нет схемы")
	}
	gz := gzip.NewWriter(w)
	buffer := bufio.NewWriterSize(gz, 64*1024)
	n := nbtWriter{w: buffer}
	n.tag(10, "")
	n.tag(3, "DataVersion")
	n.raw(int32(s.Info.DataVersion))
	n.ints("size", s.Info.Size)
	n.tag(9, "palette")
	n.raw(byte(10))
	n.raw(int32(len(s.Palette)))
	for i, name := range s.Palette {
		n.tag(8, "Name")
		n.str(name)
		if i < len(s.Properties) && len(s.Properties[i]) > 0 {
			n.tag(10, "Properties")
			keys := make([]string, 0, len(s.Properties[i]))
			for key := range s.Properties[i] {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				n.tag(8, key)
				n.str(s.Properties[i][key])
			}
			n.raw(byte(0))
		}
		n.raw(byte(0))
	}
	n.tag(9, "blocks")
	n.raw(byte(10))
	n.raw(int32(len(s.Blocks)))
	for _, b := range s.Blocks {
		n.ints("pos", [3]int{b.X, b.Y, b.Z})
		n.tag(3, "state")
		n.raw(int32(b.State))
		n.raw(byte(0))
	}
	n.tag(9, "entities")
	n.raw(byte(10))
	n.raw(int32(0))
	n.raw(byte(0))
	if n.err != nil {
		gz.Close()
		return n.err
	}
	if err := buffer.Flush(); err != nil {
		gz.Close()
		return err
	}
	return gz.Close()
}

func atomicOutput(path string, write func(io.Writer) error) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".mapart-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if err = write(f); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
func writeJSON(w io.Writer, value any) error {
	e := json.NewEncoder(w)
	e.SetIndent("", "  ")
	return e.Encode(value)
}
func WriteMaterials(w io.Writer, s *Structure) error {
	c := csv.NewWriter(w)
	if err := c.Write([]string{"block", "count", "stacks_64", "remaining"}); err != nil {
		return err
	}
	for _, m := range s.Info.Materials {
		if err := c.Write([]string{m.Block, strconv.Itoa(m.Count), strconv.Itoa(m.Stacks), strconv.Itoa(m.Remainder)}); err != nil {
			return err
		}
	}
	c.Flush()
	return c.Error()
}
func ExportStructure(path string, s *Structure) error {
	if !strings.EqualFold(filepath.Ext(path), ".nbt") {
		return fmt.Errorf("схема должна иметь расширение .nbt")
	}
	if err := atomicOutput(path, func(w io.Writer) error { return WriteStructureNBT(w, s) }); err != nil {
		return err
	}
	stem := strings.TrimSuffix(path, filepath.Ext(path))
	if err := atomicOutput(stem+"_materials.csv", func(w io.Writer) error { return WriteMaterials(w, s) }); err != nil {
		return err
	}
	if err := atomicOutput(stem+"_build.json", func(w io.Writer) error { return writeJSON(w, s.Info) }); err != nil {
		return err
	}
	return savePNG(stem+"_map.png", s.MapImage)
}

func ExportStructureTiles(path string, result *PreviewResult, opts BuildOptions) error {
	if !strings.EqualFold(filepath.Ext(path), ".zip") {
		return fmt.Errorf("набор карт должен иметь расширение .zip")
	}
	if err := opts.Validate(); err != nil {
		return err
	}
	if _, err := structureColorLookup(result); err != nil {
		return err
	}
	b := result.Image.Bounds()
	if b.Dx()%128 != 0 || b.Dy()%128 != 0 {
		return fmt.Errorf("для разбиения размер должен быть кратен 128")
	}
	return atomicOutput(path, func(w io.Writer) error {
		archive := zip.NewWriter(w)
		defer archive.Close()
		type tileInfo struct {
			File   string        `json:"file"`
			Column int           `json:"column"`
			Row    int           `json:"row"`
			Info   StructureInfo `json:"build"`
		}
		var tiles []tileInfo
		for row := 0; row < b.Dy()/128; row++ {
			for col := 0; col < b.Dx()/128; col++ {
				part := *result
				part.Image = result.Image.SubImage(image.Rect(b.Min.X+col*128, b.Min.Y+row*128, b.Min.X+(col+1)*128, b.Min.Y+(row+1)*128)).(*image.RGBA)
				s, err := BuildStructure(&part, opts)
				if err != nil {
					return err
				}
				name := fmt.Sprintf("map_x%02d_z%02d.nbt", col+1, row+1)
				entry, err := archive.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Store})
				if err != nil {
					return err
				}
				if err = WriteStructureNBT(entry, s); err != nil {
					return err
				}
				csvEntry, err := archive.Create(strings.TrimSuffix(name, ".nbt") + "_materials.csv")
				if err != nil {
					return err
				}
				if err = WriteMaterials(csvEntry, s); err != nil {
					return err
				}
				tiles = append(tiles, tileInfo{name, col + 1, row + 1, s.Info})
			}
		}
		entry, err := archive.Create("layout.json")
		if err != nil {
			return err
		}
		if err = writeJSON(entry, tiles); err != nil {
			return err
		}
		entry, err = archive.Create("README.txt")
		if err != nil {
			return err
		}
		_, err = io.WriteString(entry, "Каждая NBT — независимая карта 128x128 со служебным северным рядом (размер 128 x H x 129).\nСтройте карты отдельно, заполните и зафиксируйте каждую картографическим столом со стеклянной панелью.\nЗатем расположите карты в рамках: x слева направо, z сверху вниз, начиная с 01.\nНе склеивайте эти NBT как одну поверхность: высоты рассчитаны независимо, служебные ряды пересекаются.\nВерх изображения — север (-Z), вращение и зеркалирование меняют оттенки.\nМатериалы и размеры каждой схемы указаны в layout.json и CSV.\n")
		if err != nil {
			return err
		}
		return archive.Close()
	})
}
