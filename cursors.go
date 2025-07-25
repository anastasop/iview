package main

import (
	"image"

	draw9 "9fans.net/go/draw"
)

var (
	// lockarrow is used while waiting. Copied from samterm.
	lockarrow = &draw9.Cursor{
		Point: image.Point{-7, -7},
		White: [32]uint8{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
		Black: [32]uint8{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x0F, 0xC0, 0x0F, 0xC0,
			0x03, 0xC0, 0x07, 0xC0, 0x0E, 0xC0, 0x1C, 0xC0,
			0x38, 0x00, 0x70, 0x00, 0xE0, 0xDB, 0xC0, 0xDB},
	}
)
