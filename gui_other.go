//go:build !windows && gui

package main

func runGUI(opts RenderOptions, path string, buildOpts BuildOptions) error {
	return RunFyneGUI(opts, path, buildOpts)
}
func reportGUIStartupError(error) {}
