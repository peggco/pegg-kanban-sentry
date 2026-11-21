package main

import (
	peggplugin "github.com/peggco/pegg/pkg/plugin"
)

func main() {
	peggplugin.Serve(&Plugin{})
}
