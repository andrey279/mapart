# Linux GUI

The Linux desktop interface uses Fyne. The CLI and GUI are separate builds of the same Go package.

## Build

Install Go 1.26 or newer. On Debian or Ubuntu, install the C compiler, OpenGL headers, and desktop libraries used by Fyne:

```bash
sudo apt-get install -y build-essential pkg-config libgl1-mesa-dev \
  libxcursor-dev libxrandr-dev libxinerama-dev libxi-dev libxxf86vm-dev \
  libwayland-dev libxkbcommon-dev wayland-protocols
```

Then build with the `gui` tag:

```bash
go mod download
go build -tags gui -o mapart-gui .
```

`./build-gui-linux.sh` runs the download and build steps. It does not install system packages. For CLI-only use, `go build -o mapart .` does not require the GUI libraries.

## Run

```bash
./mapart-gui
./mapart-gui -gui -i source.png -preset pastel -x 5 -y 3
```

Running the GUI build without arguments opens MapArt Studio. The `-gui` option lets you pass an image and rendering options; a supplied image is converted on startup. Opening or dropping an image also starts conversion.

The layout and controls follow the Windows interface, with Russian labels:

- Settings on the left: size, dithering, color, masks, and build options. All rendering options, including crop, gamma, sharpening, manual masks, and custom palettes, are available.
- Preset selection plus an explicit **Применить** button. Applying a preset preserves dimensions, framing, matte color, custom files, and the terracotta switch.
- JSON profile load/save using the same format as the CLI and Windows GUI.
- Preview tabs on the right: comparison, the exact in-game map, background mask, and materials with placement instructions. **Размер 1:1** enables scrolling at original pixel size.
- Separate **Сохранить PNG…**, **Схема .nbt…**, and **ZIP по картам…** buttons. Exports are disabled after changing rendering options until the preview is updated. Build options are applied again when exporting NBT. A custom RGB palette allows PNG export only.
- **Блоки** provides 60 materials with distinct Minecraft map colours, with search and group selection. All 180 staircase shades / 60 flat colours are retained after removing 625 material duplicates. Old profile names resolve to the retained equivalent colour. The catalogue recycles visible rows; scrolling and filtering preserve hidden selections. Choose a group from the dropdown, then add or remove it without replacing other selections. **Полная палитра блоков (Java 26.3)** on the colour tab enables all 60 materials (`all_blocks` in JSON, `-all-blocks` in the CLI). **Постройка** selects flat **2D** or staircase **3D**; NBT defaults to DataVersion 5023 (Java 26.3).
- **QR-код…** in the header opens the local QR generator: URL/text, map dimensions, flat 2D or staircase 3D, square/rounded/circular elements, separate Minecraft blocks for foreground/background/corners and an optional central logo. **Обновить превью** checks the exact Minecraft colours; **Создать мап-арт** sends the result to the main editor for brush edits and PNG/NBT/ZIP export. The status reports whether the original payload still decodes after edits. Saving PNG also saves an `_original.png` image for reopening later; the QR payload is kept in the current session, outside conversion profiles.
- **Редактор исходника…** provides mouse cropping, rotation, mirroring and live brightness, contrast, saturation, gamma and temperature adjustments. Applying the editor settings converts the image again; cancelling discards them.
- **Редактор карты…** edits the converted pixels with a brush, flood fill, eyedropper, palette swatches, zoom and undo/redo. Applying edits recalculates the build and materials for PNG/NBT/ZIP export. Converting the source again replaces manual map edits. JSON profiles store conversion settings rather than painted pixels.
  Mouse wheel and **+ / −** buttons zoom in and out; the percentage selector includes 25–3200% and **Вписать** restores the full map view. Wheel zoom keeps the image point under the pointer where scrolling allows. Zoom does not change exported pixels or the brush size in blocks.

Image conversion and structure export run in the background. The settings are locked while they run; conversion errors leave the prior preview available for comparison.

Reduced map previews use smooth sampling so dithering does not turn into moire or false colours. Enlarged previews and 1:1 show sharp original pixels. This affects display only, leaving PNG/NBT exports unchanged. The landscape preset uses RGB diffusion without sharpening or automatic masks; additional materials help preserve pale sky colours and cool shadows.

On a Wayland desktop, keep `WAYLAND_DISPLAY` available; on X11, keep `DISPLAY` available. The Fyne/GLFW backend chooses a supported display route. A Wayland session may use XWayland depending on the installed GLFW and graphics stack. Check the process output for display or OpenGL errors if no window appears.

## Checks

```bash
go test ./...
go test -tags gui ./...
```

The first command checks the CLI build; the second compiles and tests the GUI build. A desktop session is required to actually display the window.
