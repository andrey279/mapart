//go:build !windows && !gui

package main

import "fmt"

func runGUI(RenderOptions, string, BuildOptions) error {
	return fmt.Errorf("GUI is not included in this build; run go build -tags gui -o mapart-gui .")
}

func reportGUIStartupError(error) {}
