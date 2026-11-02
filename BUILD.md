# Building mapartscript

Go 1.26 or newer is required by `go.mod`.

## Linux

The CLI build needs no GUI development libraries:

```bash
go build -o mapart .
go test ./...
```

For the Fyne GUI, install the OpenGL and desktop development packages listed in [GUI_LINUX.md](GUI_LINUX.md), then run:

```bash
go build -tags gui -o mapart-gui .
go test -tags gui ./...
./mapart-gui
```

`make build` builds both variants; `make build-cli` and `make build-gui` build them separately. The GUI runs under a supported Wayland or X11 display route chosen by the installed Fyne/GLFW backend. `WAYLAND_DISPLAY` or `DISPLAY` must point to the active desktop session.

## Windows

On Windows, the default build includes the native Walk GUI. Run without arguments or use `-gui` to open it:

```powershell
go build -o mapart.exe .
.\mapart.exe -gui
```

`build.ps1` builds the project's Windows executables and embeds the manifest. For cross-compiling from Linux, `CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o mapart.exe .` works without GUI system libraries.

## macOS

The CLI uses `go build -o mapart .`. The Fyne GUI uses `go build -tags gui -o mapart-gui .` and requires the platform graphics development environment.

## Docker

`docker build -t mapartscript .` builds the CLI image. It has no desktop GUI. Pass CLI arguments when running the container; see `Dockerfile` and `docker-compose.yml` for paths.
