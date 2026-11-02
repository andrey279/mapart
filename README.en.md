# MapArt Studio

A command-line image converter and desktop editor for Minecraft Java map art.
Create quantized PNG previews and vanilla structure `.nbt` builds usable with
Litematica, with flat **2D** surfaces or **3D** staircases.

[Full guide in Russian](README.md) · [Releases](https://github.com/andrey279/mapart/releases) · [Build instructions](BUILD.md) · [Issues](https://github.com/andrey279/mapart/issues)

One map is 128 × 128 pixels and one output pixel represents one surface block.
The CLI, native Windows GUI and Linux Fyne GUI use the same renderer and JSON
profiles. Local image conversion and QR generation work offline; `-links`
downloads the URLs you supply.

## Build and quick start

Go 1.26 or newer is required by `go.mod`. Supply your own input images.

```bash
git clone https://github.com/andrey279/mapart.git
cd mapart
go mod download
go build -o mapart .
./mapart -i photo.jpg -preset portrait -x 3 -y 2 -o output/art.png -nbt output/art.nbt
```

On Windows, build with `go build -o mapart.exe .` and use `.\mapart.exe` in
PowerShell. `mapart.exe` without arguments opens the native GUI;
`build.ps1` creates both `mapart.exe` and `mapart-gui.exe`.

Linux Fyne GUI needs the desktop development libraries listed in
[GUI_LINUX.md](GUI_LINUX.md):

```bash
go build -tags gui -o mapart-gui .
./mapart-gui
```

Linux/macOS CLI builds do not need these GUI libraries. macOS can build Fyne
with the `gui` tag and the platform graphics development environment; automated
macOS packages currently contain the CLI. Published binaries, when available,
are in the repository's Releases section.

## CLI capabilities

Input: PNG, JPEG, WebP, GIF (one frame), BMP or TIFF. Output: PNG and optional
build exports. Use `-i`, `-dir`, `-links` or `-qr` as mutually exclusive sources.

```bash
# Restrict a flat build to available materials.
./mapart -i logo.png -x 1 -y 1 -build-mode 2d \
  -blocks white_wool,black_wool,red_wool -nbt output/logo.nbt

# Full catalogue and independent map-tile builds.
./mapart -i photo.jpg -all-blocks -x 3 -y 2 -nbt-tiles output/tiles.zip

# Batch conversion creates PNGs; NBT needs one -i or -qr source.
./mapart -dir input -preset landscape -out-dir output/batch
./mapart -links links.txt -out-dir output/downloads

# Compare presets and inspect settings.
./mapart -i photo.jpg -compare all -out-dir output/compare
./mapart -preset portrait -save-config portrait.json
./mapart -i photo.jpg -config portrait.json -strength 0.5 -o output/art.png
./mapart -i photo.jpg -debug -o output/debug.png
./mapart -list-presets
./mapart -h
```

A URL list accepts one URL per line or `URL X Y`; `#` starts a comment line.
Directory processing is nonrecursive. A failure does not discard earlier
successful conversions, but the process exits with an error if any input fails.
Preset comparison creates images, configuration files and an HTML comparison.

Settings resolve as **preset → JSON profile → explicitly supplied flags**,
including explicit zero/false overrides. Use `-sky-cleanup=false` to disable a
boolean option. Profiles store conversion settings and asset paths, not painted
pixels, QR payloads or build-only export options.

The complete flag reference is in the [Russian guide](README.md); `-h` lists all
flags. Supported settings include:

| Area | Controls |
| --- | --- |
| Geometry | Map counts, rotation, mirroring, crop, stretch/contain/cover, nearest/bilinear/bicubic, matte colour |
| Matching | Lab/CIEDE2000 or RGB; Floyd–Steinberg, ordered or no dithering, strength and error threshold |
| Tones | Brightness, contrast, saturation, gamma, temperature, sharpening, denoise, auto-contrast and neutral colours |
| Masks | Automatic/colour/off background, tolerance, sky/haze handling, manual no-dither and detail masks |
| Materials | Wool, terracotta, natural additions, full catalogue, explicit block IDs or a custom RGB PNG palette |
| Build | Flat 2D or staircase 3D, supports, height limit, NBT DataVersion, unified NBT or tiled ZIP |
| Output | PNG, original reference, debug masks/settings, palette JSON, materials CSV and placement JSON |

Nine presets: `balanced`, `portrait`, `pastel`, `landscape`, `night`, `lineart`,
`pixel`, `flat` and `clean-sky`. The `flat` preset adjusts image processing;
choose `-build-mode 2d` separately for a flat physical build.

## GUI capabilities

Open or drop an image, choose a preset and click **«Применить»** (Apply), adjust
the settings and click **«Преобразовать»** (Convert). Processing runs in the
background. Changing rendering settings disables exports until reconversion;
supports, height and DataVersion apply at the next NBT export.

| Panel | Functions |
| --- | --- |
| Blocks | Automatic palette or individual materials, Russian name/ID search, group filtering, group add/remove and selection shortcuts; filtering preserves hidden selections |
| Size | Horizontal/vertical map counts, fit/resampling, crop and matte colour |
| Dithering | Algorithm, strength, threshold, Lab/RGB, haze suppression and neutral colours |
| Colour | Catalogue switches, brightness, contrast, saturation, gamma, temperature, sharpening, denoise and auto-contrast |
| Masks | Background detection, colour/tolerance, sky suppression, manual mask files and custom RGB palette |
| Build | 2D/3D, cobblestone supports, height limit and NBT DataVersion |

Load/save JSON profiles with the same format as the CLI. Applying a preset
preserves geometry, selected files, materials and build mode.

Preview tabs show the reference and result side by side, the reconstructed
in-game map, the background mask and materials/placement instructions.
**«Размер 1:1»** enables original-pixel viewing with scrolling. Preview smoothing
only affects display, not exported pixels. Linux also provides a materials copy
button.

**Source editor:** mouse crop, reset crop, 90-degree rotation, horizontal/vertical
mirroring and live brightness, contrast, saturation, gamma and temperature.
Apply reruns conversion; cancel discards editor changes.

**Map editor:** brush sizes 1/3/5/9/17 blocks, continuous strokes, flood fill,
eyedropper, material/shade selection, undo/redo, mouse-wheel and button zoom,
25–3200% choices, fit-to-view and scrolling. Applying edits recalculates the
build and materials for PNG/NBT/ZIP. Reconversion replaces painted edits.
Profiles do not store the edit history.

**QR generator:** text/URL, dimensions, 2D/3D, square/rounded/dot elements,
separate foreground/background/finder materials, optional centre logo sized
5–20%, preview and create actions. Codes are checked by decoding, including
after applied map edits. The URL is encoded as text, not opened. QR defaults
to one flat map in black and white wool. The generated source remains in the
current session and is saved as `_original.png` alongside PNG export.

```bash
./mapart -qr "https://example.com" -o output/qr.png -nbt output/qr.nbt
./mapart -qr "https://example.com" -x 2 -y 2 -qr-style dots \
  -qr-fg blue_wool -qr-bg white_wool -qr-finder red_wool \
  -qr-logo logo.png -qr-logo-size 15 -nbt-tiles output/qr_tiles.zip
```

## Palettes and exports

The default 16 wool colours give 48 staircase shades or 16 flat colours. The
full catalogue has 60 distinct base map colours / 180 staircase shades. Old
material aliases resolve to equivalent retained blocks. A custom RGB palette
allows PNG output but cannot provide block identities for NBT, manual material
selection or flat 2D mode. See the [catalogue data](verification/block_catalog/README.md).

A unified NBT export creates `art.nbt`, `art_materials.csv`, `art_build.json`
and `art_map.png`. The north reference row adds one block to the depth of the
build. Tiled ZIP contains independent 128 × 128 map builds, per-tile materials,
`layout.json` and `README.txt`; these are not a continuous staircase surface.
Open the NBT through Litematica and follow the placement metadata.

Default height limit is 384. Default DataVersion is 5023, for the bundled Java
26.3 catalogue. Older DataVersion choices reject incompatible materials rather
than making newer blocks available in an older game. The program does not place
blocks, connect to a Minecraft server or create `map_*.dat` game map files.

## Development

```bash
make build-cli
make pre-commit
make test
make coverage
go test -race ./...
go vet -tags gui ./...
go test -tags gui ./...
```

Normal tests use synthetic images and bundled JSON fixtures. Artwork, local
review galleries and built executables stay outside Git. GitHub Actions tests
Linux/Windows/macOS, checks Linux GUI and builds platform packages. Tags `v*`
create a draft release with ZIP packages and checksums for owner review.
Docker builds contain the CLI; see [Dockerfile](Dockerfile) and
[docker-compose.yml](docker-compose.yml).

[Contributing](CONTRIBUTING.md) · [Architecture](ARCHITECTURE.md) · [Security](SECURITY.md) · [Changelog](CHANGELOG.md)

Project code: [MIT](LICENSE). Dependency and font notices:
[THIRD_PARTY_NOTICES.txt](licenses/THIRD_PARTY_NOTICES.txt).
The project is not affiliated with Mojang or Microsoft.
