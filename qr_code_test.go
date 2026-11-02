package main

import (
	"archive/zip"
	"bytes"
	"image"
	"image/color"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestQRCodeStylesAndMinecraftStructures(t *testing.T) {
	for _, mode := range []string{"2d", "3d"} {
		for _, style := range qrStyles {
			t.Run(mode+"/"+style, func(t *testing.T) {
				q := DefaultQRCodeOptions()
				q.Text, q.Style, q.BuildMode = "https://example.com/mapart", style, mode
				q.ForegroundBlock, q.FinderBlock = "blue_wool", "red_wool"
				generated, err := GenerateQRCode(q)
				if err != nil {
					t.Fatal(err)
				}
				if generated.Source.Bounds() != image.Rect(0, 0, 128, 128) {
					t.Fatal("wrong map dimensions")
				}
				if !bytes.Equal(generated.Source.Pix, generated.Preview.Image.Pix) {
					t.Fatal("QR module colours changed during quantization")
				}
				s, err := BuildStructure(generated.Preview, DefaultBuildOptions())
				if err != nil {
					t.Fatal(err)
				}
				if err := VerifyQRCode(s.MapImage, q.Text); err != nil {
					t.Fatal("Minecraft colours cannot be read:", err)
				}
				if s.Info.BuildMode != mode || s.Info.SurfaceBlocks != 128*128 {
					t.Fatal("incorrect structure geometry")
				}
				if mode == "2d" && (s.Info.Size[1] != 1 || s.Info.SupportBlocks != 0) {
					t.Fatal("flat QR has raised blocks")
				}
				if mode == "3d" && s.Info.Size[1] <= 1 {
					t.Fatal("3D QR did not produce a staircase")
				}
				exact, err := qrExactMapImage(generated.Preview)
				if err != nil || !bytes.Equal(exact.Pix, s.MapImage.Pix) {
					t.Fatal("QR verification disagrees with exported map colours", err)
				}
			})
		}
	}
}

func TestQRCodeQuietZoneRectangularLayoutAndUnicode(t *testing.T) {
	q := DefaultQRCodeOptions()
	q.MapsX, q.Text = 2, "https://example.org/путь?имя=Майнкрафт&x=1"
	q.ForegroundBlock, q.FinderBlock = "black_terracotta", "blue_terracotta"
	generated, err := GenerateQRCode(q)
	if err != nil {
		t.Fatal(err)
	}
	bg, _ := qrBlockColour(q.BackgroundBlock, "Normal")
	w, h, span := 256, 128, generated.Modules*generated.ModulePixels
	x0, y0 := (w-span)/2, (h-span)/2
	if x0 < 4*generated.ModulePixels || y0 < 4*generated.ModulePixels {
		t.Fatal("quiet zone is too small")
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if !image.Pt(x, y).In(image.Rect(x0, y0, x0+span, y0+span)) && generated.Source.RGBAAt(x, y) != bg {
				t.Fatal("QR border contains marks")
			}
		}
	}
	if err := VerifyQRCode(generated.Preview.Image, q.Text); err != nil {
		t.Fatal("Unicode payload changed", err)
	}
}

func TestQRCodeLogoAndPNGNBTTileExports(t *testing.T) {
	dir := t.TempDir()
	logo := image.NewRGBA(image.Rect(0, 0, 48, 24))
	for y := 0; y < 24; y++ {
		for x := 0; x < 48; x++ {
			if x > 4 && y > 3 && y < 20 {
				logo.SetRGBA(x, y, color.RGBA{20, 140, 180, 255})
			}
		}
	}
	logoPath := filepath.Join(dir, "logo.png")
	if err := savePNG(logoPath, logo); err != nil {
		t.Fatal(err)
	}
	q := DefaultQRCodeOptions()
	q.Text, q.LogoPath, q.MapsX, q.MapsY, q.Style = "https://example.org/qr-with-logo", logoPath, 2, 2, "rounded"
	generated, err := GenerateQRCode(q)
	if err != nil {
		t.Fatal(err)
	}
	s, err := BuildStructure(generated.Preview, DefaultBuildOptions())
	if err != nil {
		t.Fatal(err)
	}
	if err := ExportStructure(filepath.Join(dir, "qr.nbt"), s); err != nil {
		t.Fatal(err)
	}
	mapPNG, err := decodeLocalImage(filepath.Join(dir, "qr_map.png"))
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyQRCode(mapPNG, q.Text); err != nil {
		t.Fatal("exported map cannot be scanned", err)
	}
	if err := savePreviewResult(generated.Preview, filepath.Join(dir, "qr.png"), generated.Options, false); err != nil {
		t.Fatal(err)
	}
	restored, err := decodeLocalImage(filepath.Join(dir, "qr_original.png"))
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyQRCode(restored, q.Text); err != nil {
		t.Fatal("saved QR source cannot be scanned", err)
	}
	archive := filepath.Join(dir, "qr.zip")
	if err := ExportStructureTiles(archive, generated.Preview, DefaultBuildOptions()); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.OpenReader(archive)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	nbtCount := 0
	for _, f := range zr.File {
		if strings.HasSuffix(f.Name, ".nbt") {
			nbtCount++
		}
	}
	if nbtCount != 4 {
		t.Fatal("wrong number of NBT tiles", nbtCount)
	}
}

func TestQRCodeInvalidInputAndUnreadableEdits(t *testing.T) {
	q := DefaultQRCodeOptions()
	q.Text = "https://example.com"
	for name, edit := range map[string]func(*QRCodeOptions){
		"empty":           func(q *QRCodeOptions) { q.Text = "  " },
		"colour":          func(q *QRCodeOptions) { q.ForegroundBlock = "white_wool" },
		"finder":          func(q *QRCodeOptions) { q.FinderBlock = "white_wool" },
		"block":           func(q *QRCodeOptions) { q.ForegroundBlock = "missing_block" },
		"multiple blocks": func(q *QRCodeOptions) { q.ForegroundBlock = "black_wool,blue_wool" },
		"size":            func(q *QRCodeOptions) { q.MapsX = 0 },
		"too many maps":   func(q *QRCodeOptions) { q.MapsX, q.MapsY = 17, 16 },
		"style":           func(q *QRCodeOptions) { q.Style = "unknown" },
		"mode":            func(q *QRCodeOptions) { q.BuildMode = "volume" },
		"missing logo":    func(q *QRCodeOptions) { q.LogoPath = filepath.Join(t.TempDir(), "missing.png") },
		"logo size":       func(q *QRCodeOptions) { q.LogoPercent = 30 },
		"too dense":       func(q *QRCodeOptions) { q.Text = strings.Repeat("abcdefghij", 100) },
	} {
		t.Run(name, func(t *testing.T) {
			bad := q
			edit(&bad)
			if _, err := GenerateQRCode(bad); err == nil {
				t.Fatal("invalid QR request succeeded")
			}
		})
	}
	generated, err := GenerateQRCode(q)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyQRCode(generated.Preview.Image, "another payload"); err == nil {
		t.Fatal("wrong payload passed verification")
	}
	bad := *generated.Preview
	bad.Image = image.NewRGBA(bad.Image.Bounds())
	if status := qrReadabilityStatus(&bad, nil, q.Text); !strings.Contains(status, "не читается") {
		t.Fatal("destroyed QR is reported as readable")
	}
	if _, err := studioSourceImage(generatedQRSource, generated.Source); err != nil {
		t.Fatal("virtual source cannot be opened", err)
	}
}

func TestCLIQRCreatesScannableMapAndStructure(t *testing.T) {
	dir := t.TempDir()
	text := "https://example.com/?map=minecraft"
	pngPath, nbtPath, zipPath := filepath.Join(dir, "code.png"), filepath.Join(dir, "code.nbt"), filepath.Join(dir, "code.zip")
	args := []string{"-qr", text, "-qr-style", "dots", "-qr-fg", "blue_wool", "-qr-finder", "red_wool", "-o", pngPath, "-nbt", nbtPath, "-nbt-tiles", zipPath}
	req, err := parseCLI(args, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if req.options.BuildMode != "2d" || req.options.MapsX != 1 || req.options.MapsY != 1 || req.options.Dither != "none" {
		t.Fatal("unsafe QR defaults", req.options)
	}
	if err := runCLI(args); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{pngPath, nbtPath, zipPath} {
		info, err := os.Stat(path)
		if err != nil || info.Size() == 0 {
			t.Fatal("missing export", path, err)
		}
	}
	img, err := decodeLocalImage(filepath.Join(dir, "code_map.png"))
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyQRCode(img, text); err != nil {
		t.Fatal(err)
	}
	for _, extra := range [][]string{{"-i", "picture.png"}, {"-dir", dir}, {"-compare", "all"}, {"-gui"}} {
		bad := append(append([]string(nil), args...), extra...)
		if err := runCLI(bad); err == nil {
			t.Fatal("incompatible QR sources/options accepted", extra)
		}
	}
	req, err = parseCLI([]string{"-qr", text, "-x", "2", "-y", "3", "-build-mode", "3d"}, io.Discard)
	if err != nil || req.qr.MapsX != 2 || req.qr.MapsY != 3 || req.options.BuildMode != "3d" {
		t.Fatal("explicit QR geometry ignored", req, err)
	}
}
