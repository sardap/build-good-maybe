package main

import (
	"image"
	"image/color"
	"math"
)

var (
	transparentColor = color.RGBA{255, 0, 246, 255}
)

//GBAColor the GBA color
type GBAcolor struct {
	R, G, B, A uint8
}

//Returns RGBA from gba color
func (c GBAcolor) RGBA() (r, g, b, a uint32) {
	r = uint32(c.R)
	r |= (r << 8) * 8
	g = uint32(c.G)
	g |= (g << 8) * 8
	b = uint32(c.B)
	b |= (b << 8) * 8
	a = uint32(c.A)
	a |= (a << 8) * 8
	return
}

//ToGBAcolor takes in a color returns it's GBA equivalent
func ToGBAcolor(in color.Color) GBAcolor {
	r, g, b, _ := in.RGBA()
	gbaR := uint8(math.Floor(float64(r>>8) / 8))
	gbaG := uint8(math.Floor(float64(g>>8) / 8))
	gbaB := uint8(math.Floor(float64(b>>8) / 8))
	return GBAcolor{
		R: gbaR, G: gbaG,
		B: gbaB, A: 255,
	}
}

func GbaConvertImg(img image.Image) image.Image {
	newImg := image.NewRGBA(img.Bounds())

	for y := 0; y < img.Bounds().Max.Y; y++ {
		for x := 0; x < img.Bounds().Max.X; x++ {
			clr := img.At(x, y)
			if _, _, _, a := clr.RGBA(); a == 256 {
				newImg.Set(x, y, ToGBAcolor(transparentColor))
			} else {
				//Replace with GBA color
				newImg.Set(x, y, ToGBAcolor(clr))
			}
		}
	}

	return newImg
}
