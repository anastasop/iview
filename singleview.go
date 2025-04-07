package main

import (
	"fmt"
	"image"
	"log"

	draw9 "9fans.net/go/draw"
)

// SingleView is a View that show single images at large scale.
type SingleView struct {
	icons      []*Icon
	iconsCache CachedSlice[*IconImage]
	at         int
	area       image.Rectangle
	showInfo   bool

	dctl *DisplayControl
}

func NewSingleView(icons []*Icon, at int, r image.Rectangle) *SingleView {
	return &SingleView{
		icons: icons,
		at:    at,
		area:  r,
	}
}

func (sv *SingleView) resetCache() {
	if sv.iconsCache != nil {
		sv.iconsCache.Free()
	}
	images := NewIconImages(sv.icons, func(img image.Image) (*draw9.Image, error) {
		return FitBest(sv.dctl.display, img, sv.area)
	})
	sv.iconsCache = NewCachedSlicePaged[*IconImage]("single", images, 2)
}

func (sv *SingleView) Connect(dctl *DisplayControl) {
	sv.dctl = dctl
	sv.resetCache()
}

func (sv *SingleView) Attach(r image.Rectangle) {
	if r.Eq(sv.area) {
		return
	}

	sv.dctl.showWaitingAndCall(func() {
		sv.dctl.cls()
		sv.area = r
		sv.resetCache()
	})
}

func (sv *SingleView) Free() {
	sv.iconsCache.Free()
}

func (sv *SingleView) Handle() View {
	bt2menu := &draw9.Menu{
		Item: []string{"info", "mark", "plumb", "back"},
	}

	dctl := sv.dctl
	sv.paint(dctl)
	for {
		select {
		case err := <-dctl.errch:
			log.Printf("display: %v", err)
		case k := <-dctl.kctl.C:
			switch k {
			case 'q', 'b', escKey: // back
				return nil
			case leftArrowKey: // prev image
				if sv.at > 0 {
					sv.at--
					sv.paint(dctl)
				}
			case rightArrowKey: // next image
				if sv.at < sv.iconsCache.Len()-1 {
					sv.at++
					sv.paint(dctl)
				}
			case 'i': // info
				sv.showInfo = !sv.showInfo
				sv.paint(dctl)
			case 'm': // mark
				if icon, ok := sv.iconsCache.At(sv.at); ok {
					icon.ToggleMarked()
					sv.paint(dctl)
				}
			case 'p': // plumb
				if icon, ok := sv.iconsCache.At(sv.at); ok {
					plumbImage(icon.path)
				}
			}
		case dctl.mctl.Mouse = <-dctl.mctl.C:
			switch dctl.mctl.Mouse.Buttons {
			case 1: // prev image
				if sv.at > 0 {
					sv.at--
					sv.paint(dctl)
				}
			case 2: // view menu
				switch draw9.MenuHit(2, dctl.mctl, bt2menu, nil) {
				case 0: // info
					sv.showInfo = !sv.showInfo
					sv.paint(dctl)
				case 1: // mark
					if icon, ok := sv.iconsCache.At(sv.at); ok {
						icon.ToggleMarked()
						sv.paint(dctl)
					}
				case 2: // plumb
					if icon, ok := sv.iconsCache.At(sv.at); ok {
						plumbImage(icon.path)
					}
				case 3: // back
					return nil
				}
			case 4: // next image
				if sv.at < sv.iconsCache.Len()-1 {
					sv.at++
					sv.paint(dctl)
				}
			}
		case <-dctl.mctl.Resize:
			if err := dctl.display.Attach(draw9.RefNone); err != nil {
				log.Fatalf("display: failed to attach: %v", err)
			}
			sv.Attach(dctl.display.Image.Bounds())
			sv.paint(dctl)
		}
	}
}

func (sv *SingleView) paint(dctl *DisplayControl) {
	dctl.display.Image.Draw(dctl.display.Image.Bounds(), dctl.bgColor, nil, image.Point{})

	var icon *IconImage
	var ok bool
	var img *draw9.Image
	var err error
	dctl.showWaitingAndCall(func() {
		if icon, ok = sv.iconsCache.At(sv.at); ok {
			img, err = icon.ForDisplay()
		}
	})
	if err != nil {
		log.Printf("singleView: image not ready: %v", err)
		return
	}

	font := dctl.display.Font
	window := dctl.display.Image

	imgR := bestFit(sv.area, img.Bounds())
	var lines []image.Point
	var text []string
	if sv.showInfo {
		lines = append(lines, sv.area.Min)
		text = append(text, fmt.Sprintf("%d/%d %v %s",
			sv.at+1, sv.iconsCache.Len(), img.Bounds().Max, icon.path))
		if icon.exifInfo != "" {
			lines = append(lines, lines[len(lines)-1].Add(image.Point{0, font.Height}))
			text = append(text, icon.exifInfo)
		}
		imgR.Min.Y += (len(lines) + 1) * font.Height
	}

	window.Draw(imgR, img, nil, image.Point{})
	if icon.marked {
		mr := image.Rect(window.Bounds().Max.X-50, window.Bounds().Min.Y,
			window.Bounds().Max.X, window.Bounds().Min.Y+font.Height)
		window.Draw(mr, dctl.borderColor, nil, image.Point{})
	}
	for i := range lines {
		window.String(lines[i], dctl.fontColor, image.Point{}, font, text[i])
	}

	if err := dctl.display.Flush(); err != nil {
		log.Printf("display: flush: %v", err)
	}
}
