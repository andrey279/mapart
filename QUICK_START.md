# Quick start

Install Go 1.26 or newer and open a terminal in the project directory.

```bash
go mod download
go build -o mapart .
./mapart -i /path/to/your/image.png -x 3 -y 2 -o output/art.png -nbt output/art.nbt
```

Supply your own PNG or JPEG. Input images and generated artwork are kept out
of Git. On Windows use `go build -o mapart.exe .`; launching `mapart.exe`
without arguments opens the native GUI. See [BUILD.md](BUILD.md) for all platforms.

For the Linux desktop interface, install the libraries in [GUI_LINUX.md](GUI_LINUX.md):

```bash
go build -tags gui -o mapart-gui .
./mapart-gui
```

Useful development commands:

```bash
make build-cli     # CLI only; no desktop libraries on Linux/macOS
make build-gui     # Fyne GUI
make test          # Normal unit and integration tests
make pre-commit    # Formatting check, vet, short tests
make coverage     # coverage.out and coverage.html
make benchmark    # Benchmarks
make fmt          # Format Go sources
```

The optional `./setup.sh` downloads dependencies, verifies them, builds the CLI,
runs checks and enables the local Git hook. It does not install global tools.

For Docker:

```bash
docker build -t mapartscript .
mkdir -p input output
# Place your image in input/image.png first.
docker run --rm -v "$PWD/input:/workspace/input:ro" -v "$PWD/output:/workspace/output" \
  mapartscript -i /workspace/input/image.png -o /workspace/output/art.png
```

See [README.md](README.md), [README.en.md](README.en.md),
[CONTRIBUTING.md](CONTRIBUTING.md) and [GITHUB_UPLOAD.md](GITHUB_UPLOAD.md).
