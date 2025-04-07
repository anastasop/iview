package main

import (
	"image"
	"log"
	"slices"

	draw9 "9fans.net/go/draw"
)

// MarkedView is a View that show the marked images as thumbnails.
type MarkedView struct {
	icons      []*Icon
	iconsCache CachedSlice[*IconImage]
	offset     *Offset
	pageSize   int

	dctl *DisplayControl
}

func NewMarkedView(icons []*Icon, grid *Grid, pageSize int) *MarkedView {
	if pageSize == 0 {
		pageSize = grid.Area()
	}
	return &MarkedView{
		icons:    icons,
		offset:   NewOffset(grid, len(icons)),
		pageSize: pageSize,
	}
}

func (mv *MarkedView) Connect(dctl *DisplayControl) {
	mv.dctl = dctl
	if mv.iconsCache != nil {
		mv.iconsCache.Free()
	}
	images := NewIconImages(mv.icons, func(img image.Image) (*draw9.Image, error) {
		return FitFast(dctl.display, img, image.Rectangle{image.Point{}, mv.offset.grid.iconSize})
	})
	mv.iconsCache = NewCachedSlicePaged[*IconImage]("marked", images, mv.pageSize)
}

func (mv *MarkedView) Attach(r image.Rectangle) {
	if r.Eq(mv.offset.grid.area) {
		return
	}
	mv.offset.grid.Attach(r)
}

func (mv *MarkedView) Free() {
	mv.iconsCache.Free()
}

func (mv *MarkedView) Handle() View {
	bt2menu := &draw9.Menu{
		Item: []string{"mark", "plumb", "", "prev page", "next page", "", "back"},
	}

	dctl := mv.dctl
	mv.paint(dctl)
	for {
		select {
		case err := <-dctl.errch:
			log.Printf("display: %v", err)
		case k := <-dctl.kctl.C:
			switch k {
			case 'q', 'b', escKey: // back
				return nil
			case upArrowKey: // scroll up
				mv.offset.MoveUpRow()
				mv.paint(dctl)
			case downArrowKey: // scroll down
				mv.offset.MoveDownRow()
				mv.paint(dctl)
			case leftArrowKey: // prev page
				mv.offset.GotoPage(mv.offset.CurrentPage() - 1)
				mv.paint(dctl)
			case rightArrowKey: // next page
				mv.offset.GotoPage(mv.offset.CurrentPage() + 1)
				mv.paint(dctl)
			}
		case dctl.mctl.Mouse = <-dctl.mctl.C:
			switch dctl.mctl.Mouse.Buttons {
			case 1: // select image
				if i, ok := mv.offset.At(dctl.mctl.Mouse.Point); ok {
					return NewSingleView(mv.icons, i, mv.offset.grid.area)
				}
			case 2: // view menu
				switch draw9.MenuHit(2, dctl.mctl, bt2menu, nil) {
				case 0: // mark
					if i, ok := mv.offset.At(dctl.mctl.Mouse.Point); ok {
						if icon, ok := mv.iconsCache.At(i); ok {
							icon.ToggleMarked()
						}
					}
					mv.paint(dctl)
				case 1: // plumb
					if i, ok := mv.offset.At(dctl.mctl.Mouse.Point); ok {
						if icon, ok := mv.iconsCache.At(i); ok {
							plumbImage(icon.path)
						}
					}
				case 2:
					// nop
				case 3: // prev page
					mv.offset.GotoPage(mv.offset.CurrentPage() - 1)
					mv.paint(dctl)
				case 4: // next page
					mv.offset.GotoPage(mv.offset.CurrentPage() + 1)
					mv.paint(dctl)
				case 5:
					// nop
				case 6:
					return nil
				}
			case 4: // mark image
				if i, ok := mv.offset.At(dctl.mctl.Mouse.Point); ok {
					if icon, ok := mv.iconsCache.At(i); ok {
						icon.ToggleMarked()
					}
				}
				mv.paint(dctl)
			case scrollWheelUp: // scroll up
				mv.offset.MoveUpRow()
				mv.paint(dctl)
			case scrollWheelDown: // scroll down
				mv.offset.MoveDownRow()
				mv.paint(dctl)
			}
		case <-dctl.mctl.Resize:
			if err := dctl.display.Attach(draw9.RefNone); err != nil {
				log.Fatalf("display: failed to attach: %v", err)
			}
			mv.Attach(dctl.display.Image.Bounds())
			mv.paint(dctl)
		}
	}
}

func (mv *MarkedView) paint(dctl *DisplayControl) {
	dctl.showWaitingAndCall(func() {
		from, to := mv.offset.Visible()
		images := slices.Collect(Get(mv.iconsCache, from, to))
		paintIcons(dctl, mv.offset.grid, images)
	})
}
