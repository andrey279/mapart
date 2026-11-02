# Architecture

MapArt Studio is a Go application. Its CLI, native Windows interface and Fyne
interface share rendering, image editing, QR generation and NBT export code.
The current application lives in the root `main` package; there is no separate
`cmd/`, `internal/` or public library package.

## Processing

```text
Local image / downloaded image / generated QR
  → rotation, mirroring, crop and resizing
  → tone correction, denoise and sharpening
  → automatic masks and manual mask overrides
  → block palette selection and quantization
  → PreviewResult
      → PNG and optional diagnostic images
      → BuildStructure
          → height layout, materials and in-game map preview
          → vanilla structure NBT or independent map tiles in ZIP
```

`RenderOptions` contains image conversion settings. Resolution is one map per
128 × 128 output pixels. Options are resolved in the order preset → JSON profile
→ explicitly supplied CLI flags, then validated. Profiles retain explicit zero
and false values. Profile asset paths are resolved relative to the JSON file.

`BuildOptions` is separate: supports, maximum height and NBT DataVersion are
export settings and are not embedded in `RenderOptions` profiles.

## Source map

| Files | Responsibility |
| --- | --- |
| `main.go` | Entry point, base wool palette, colour conversion and background detection |
| `cli.go` | Flag parsing, single/batch/URL processing, preset comparison |
| `options.go` | Settings, validation, presets and profile load/save |
| `preprocess.go` | Geometric preparation, tones, filters and mask transformations |
| `render.go` | Image decoding, effective palettes, preview pipeline and PNG diagnostics |
| `quantize.go`, `dither.go`, `sky.go` | Quantization, dithering, background and sky handling |
| `block_palette.go`, `block_picker.go` | Materials, legacy aliases and block selection/search |
| `block_catalog_generated.go` | Generated additional materials and alias table |
| `structure.go` | Height calculation, block placement, material counts, NBT/CSV/JSON/ZIP export |
| `image_editor.go` | Shared transformations, pixel editor, stroke history, fill and zoom helpers |
| `qr_code.go` | QR generation, styling, logo placement and decode verification |
| `gui_windows*.go` | Native Windows GUI, source/map editors and QR dialog |
| `gui_fyne*.go` | Fyne GUI, source/map editors, QR dialog and preview layout |
| `gui_other.go`, `gui_stub.go` | Dispatch for non-Windows builds with/without the GUI tag |

The Windows files use `//go:build windows`. Fyne uses
`//go:build !windows && gui`; the non-GUI stub uses `!windows && !gui`.
Consequently the ordinary Linux/macOS CLI build does not need OpenGL or desktop
libraries. Windows uses the native Walk interface without the `gui` tag.

## Palettes and quantization

The default wool palette contains 16 base colours and three shades per colour.
The complete deduplicated catalogue has 60 materials and 180 staircase shades.
`2d` retains the normal shade of each selected material. Selection, groups and
search use stable material IDs; legacy duplicate names map to retained materials
of the same map colour.

Matching supports CIEDE2000 in Lab and RGB distance. Dithering supports
Floyd–Steinberg, ordered patterns and no dithering. Automatic masks suppress
dithering in selected background/sky regions. The manual no-dither mask is
applied before the detail mask, so the detail mask can restore dithering.
A custom RGB palette supports PNG output; it does not provide block identities
for NBT and cannot be combined with manual block selection or flat 2D mode.

Generator inputs and regression fixtures are in `verification/block_catalog/`.
Regenerate the catalogue with:

```bash
python3 verification/block_catalog/generate.py
gofmt -w block_catalog_generated.go
```

## Build and exports

`PreviewResult` contains the prepared reference image, processed image, mask,
quantized pixels, palette and build mode. `BuildStructure` resolves pixels to
block colours and computes surface heights, required block states, optional
supports and a north reference row. The result includes material counts and a
reconstructed Minecraft map image. Height and block-version limits are checked
before export.

A single NBT export writes gzip-compressed vanilla structure NBT plus materials
CSV, build information JSON and a map PNG. The tiled ZIP contains independent
128 × 128 map builds with a north row, per-tile materials CSV, `layout.json` and
`README.txt`. Tile heights are independent, so tiles are not a single continuous
surface. NBT/CSV/JSON and ZIP writes use temporary files and rename; PNG writes
are separate and a multi-file export is not one atomic transaction.

## GUI and editing

Image conversion and structure work run outside the UI thread. Completion is
scheduled on the UI thread; window state tracks whether conversion is busy,
whether the preview is stale, and whether the window has closed. Rendering
changes disable export until the next conversion. Build-only settings are read
again when exporting.

Source editing changes conversion options, then reruns the pipeline. Map editing
works on a copy of the quantized pixels and applies it after confirmation.
Undo/redo records changed pixels per action rather than copying the full image
on every stroke. Applying map edits recalculates structure/materials. A fresh
conversion replaces those edits. Profiles store conversion settings, not the
pixel edit history or QR payload.

QR generation shares the same block catalogue and validates the generated
image by decoding it. The supplied URL is encoded as text and is not opened.
The image remains available in the active session and can be exported through
the normal PNG/NBT/ZIP path.

## Network and checks

The application's explicit network input is `-links`, implemented with Go's HTTP
client and a 30-second request timeout. Normal local image conversion and QR
generation do not need a server. Runtime dependencies are listed in `go.mod`.

Normal tests use synthetic images and bundled JSON fixtures. The optional
`internetreview` and `renderreview` tags require local artwork omitted from Git.
See [CONTRIBUTING.md](CONTRIBUTING.md) and [BUILD.md](BUILD.md) for validation
commands. GitHub Actions tests Linux, Windows and macOS and prepares release
packages; a `v*` tag produces a draft release for owner review.
