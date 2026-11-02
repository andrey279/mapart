# Contributing

Use Go 1.26 or newer, as specified in `go.mod`. Clone your fork using the URL
from its **Code** menu, then run these commands from the project directory:

```bash
go mod download
go mod verify
make build-cli
make pre-commit
```

`./setup.sh` performs the same setup and enables the optional `.githooks`
pre-commit hook when run inside a Git repository. It does not install global
tools or system packages. See [BUILD.md](BUILD.md) for Windows and GUI builds.

Keep changes focused. Include a description of the problem, the resulting
behavior and relevant validation in your pull request. Add tests when behavior
changes. Use `make fmt` to format Go code; `make pre-commit` checks formatting,
runs `go vet` and runs the short test suite. `make test` runs all normal tests;
`make coverage` writes the coverage report. `make lint` uses golangci-lint v2
when installed, otherwise it runs `go vet`.

For changes to the Fyne interface, also run:

```bash
go vet -tags gui ./...
go test -tags gui ./...
```

Linux GUI builds need the desktop development libraries in [GUI_LINUX.md](GUI_LINUX.md).
On headless Linux, use `xvfb-run -a go test -tags gui ./...`.

Normal tests use synthetic images and the JSON fixtures in
`verification/block_catalog/`; keep those fixtures in the repository.
The `internetreview` and `renderreview` build tags enable optional local
artwork reviews. Those images and their output galleries are excluded from
the source distribution, so these optional tests need your local review files.

The module is named `mapart` and builds as an application. A GitHub repository
URL is not required in `go.mod` to clone and build it.

Do not commit credentials, downloaded artwork, generated output or binaries.
For bugs, include the operating system, build command or release version,
reproduction steps and expected behavior. See [SECURITY.md](SECURITY.md) for
private security reports and [ARCHITECTURE.md](ARCHITECTURE.md) for internals.

After changing dependencies, run `go mod download` and
`python3 scripts/update_notices.py` to refresh the notices included in release packages.
