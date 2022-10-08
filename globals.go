package main

import "os"

var (
	gritCmdPath  = "grit"
	asepritePath = "aseprite"
	bmp2gbaPath  = "bmp2gba.com"
	gfxCache     *GraphicsCache
)

func init() {
	if path := os.Getenv("GRIT_PATH"); path != "" {
		gritCmdPath = path
	}
	if path := os.Getenv("ASEPRITE_PATH"); path != "" {
		asepritePath = path
	}
	if path := os.Getenv("BMP2GBA_PATH"); path != "" {
		bmp2gbaPath = path
	}
}
