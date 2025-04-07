package main

import (
	"image"
	"log"
)

// paintIcons draws the grid of icons.
func paintIcons(dctl *DisplayControl, grid *Grid, icons []*IconImage) {
	dctl.display.Image.Draw(dctl.display.Image.Bounds(), dctl.bgColor, nil, image.Point{})

	pad := image.Pt(grid.padding, grid.padding)
	iconSize := grid.iconSize
	iconRect := image.Rect(0, 0, iconSize.X, iconSize.Y)
	zp := image.Point{}

	ir := grid.PaintableArea()
	pin := ir.Min
	nextIcon := 0
	for nextIcon < len(icons) && pin.Add(iconSize).In(ir) {
		for nextIcon < len(icons) && pin.Add(iconSize).In(ir) {
			icon := icons[nextIcon]
			if img, err := icon.ForDisplay(); err == nil {
				dr := center(iconRect.Add(pin).Add(pad), img.Bounds())
				dctl.display.Image.Draw(dr, img, nil, zp)
				if icon.marked {
					dctl.display.Image.Border(dr, pad.X, dctl.borderColor, zp)
				}
			} else {
				log.Printf("paintIcons: image not ready: %v", err)
			}
			nextIcon++
			pin.X += iconSize.X + pad.X
		}
		pin.Y += iconSize.Y + pad.Y
		pin.X = ir.Min.X
	}
	if err := dctl.display.Flush(); err != nil {
		log.Printf("display: flush: %v", err)
	}
}
