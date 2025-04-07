package main

import "image"

// center assume sr fits in dr and centers it inside dr. If this is not the case, it returns dr.
func center(dr, sr image.Rectangle) image.Rectangle {
	dx := dr.Dx() - sr.Dx()
	dy := dr.Dy() - sr.Dy()

	if dx < 0 || dy < 0 {
		return dr
	}

	return sr.Sub(sr.Min).Add(dr.Min.Add(image.Pt(dx, dy).Div(2)))
}

// bestFit scales down sr to fix in dr. If sr already fits, it is not scaled up.
func bestFit(dr, sr image.Rectangle) image.Rectangle {
	var r image.Rectangle
	if sr.Dx() <= dr.Dx() && sr.Dy() <= dr.Dy() {
		r = sr
	} else {
		scale := max(float32(sr.Dy())/float32(dr.Dy()), float32(sr.Dx())/float32(dr.Dx()))
		r.Max.X = int(float32(sr.Dx()) / scale)
		r.Max.Y = int(float32(sr.Dy()) / scale)
	}
	return center(dr, r)
}

// intCeil returns the ceiling of a/b
func intCeil(a, b int) int {
	n := a / b
	if a%b > 0 {
		n++
	}
	return n
}
