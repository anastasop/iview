package main

import (
	"image"
	"log"
	"slices"

	draw9 "9fans.net/go/draw"
)

// IconsView handles a display of icons over a grid.
// One screen of icons is called a page. It provides operations to
// scroll pages and mark icons. It also maintains an icon cache
// for smoother UI.
type IconsView struct {
	icons           []*Icon
	iconsCache      CachedSlice[*IconImage]
	offset          *Offset
	pageSize        int
	pagesWithMarked []int // the pages with marked icons. Used for moving up/down.

	dctl *DisplayControl
}

// NewIconsView returns an IconsView for the icons and the grid.
func NewIconsView(icons []*Icon, grid *Grid, pageSize int) *IconsView {
	if pageSize == 0 {
		pageSize = grid.Area()
	}
	return &IconsView{
		icons:    icons,
		offset:   NewOffset(grid, len(icons)),
		pageSize: pageSize,
	}
}

func (iv *IconsView) Connect(dctl *DisplayControl) {
	iv.dctl = dctl
	if iv.iconsCache != nil {
		iv.iconsCache.Free()
	}
	images := NewIconImages(iv.icons, func(img image.Image) (*draw9.Image, error) {
		return FitFast(iv.dctl.display, img,
			image.Rectangle{image.Point{}, iv.offset.grid.iconSize})
	})
	iv.iconsCache = NewCachedSlicePaged[*IconImage]("icons", images, iv.pageSize)
}

func (iv *IconsView) Attach(r image.Rectangle) {
	if r.Eq(iv.offset.grid.area) {
		return
	}
	iv.offset.grid.Attach(r)
	iv.resetPagesWithMarked()
}

func (iv *IconsView) Free() {
	iv.iconsCache.Free()
}

// handle handles mouse and keyboard actions
func (iv *IconsView) Handle() View {
	bt2menu := &draw9.Menu{
		Item: []string{"mark", "plumb", "", "prev page", "next page", "",
			"marked", "prev mark", "next mark", "", "exit"},
	}

	dctl := iv.dctl
	iv.paint(dctl)
	for {
		select {
		case err := <-dctl.errch:
			log.Printf("display: %v", err)
		case k := <-dctl.kctl.C:
			switch k {
			case 'q', 'e', escKey: // exit
				return nil
			case upArrowKey: // scroll up
				iv.offset.MoveUpRow()
				iv.paint(dctl)
			case downArrowKey: // scroll down
				iv.offset.MoveDownRow()
				iv.paint(dctl)
			case leftArrowKey: // prev page
				iv.offset.GotoPage(iv.offset.CurrentPage() - 1)
				iv.paint(dctl)
			case rightArrowKey: // next page
				iv.offset.GotoPage(iv.offset.CurrentPage() + 1)
				iv.paint(dctl)
			}
		case dctl.mctl.Mouse = <-dctl.mctl.C:
			switch dctl.mctl.Mouse.Buttons {
			case 1: // select image
				if i, ok := iv.offset.At(dctl.mctl.Mouse.Point); ok {
					return NewSingleView(iv.icons, i, iv.offset.grid.area)
				}
			case 2: // view menu
				switch draw9.MenuHit(2, dctl.mctl, bt2menu, nil) {
				case 0: // mark
					if i, ok := iv.offset.At(dctl.mctl.Mouse.Point); ok {
						iv.toggleMarked(i)
						iv.paint(dctl)
					}
				case 1: // plumb
					if i, ok := iv.offset.At(dctl.mctl.Mouse.Point); ok {
						if icon, ok := iv.iconsCache.At(i); ok {
							plumbImage(icon.path)
						}
					}
				case 2: // nop
				case 3: // prev page
					iv.offset.GotoPage(iv.offset.CurrentPage() - 1)
					iv.paint(dctl)
				case 4: // next page
					iv.offset.GotoPage(iv.offset.CurrentPage() + 1)
					iv.paint(dctl)
				case 5: // nop
				case 6: // marked
					if marked := iv.collectMarkedIcons(); len(marked) > 0 {
						return NewMarkedView(marked, iv.offset.grid, iv.offset.grid.Area())
					}
				case 7: // prev mark
					iv.moveUpToNextPageWithMarked()
					iv.paint(dctl)
				case 8: // next mark
					iv.moveDownToNextPageWithMarked()
					iv.paint(dctl)
				case 9: // nop
				case 10: // exit
					return nil
				}
			case 4: // mark image
				if i, ok := iv.offset.At(dctl.mctl.Mouse.Point); ok {
					iv.toggleMarked(i)
					iv.paint(dctl)
				}
			case scrollWheelUp: // scroll up
				iv.offset.MoveUpRow()
				iv.paint(dctl)
			case scrollWheelDown: // scroll down
				iv.offset.MoveDownRow()
				iv.paint(dctl)
			}
		case <-dctl.mctl.Resize:
			if err := dctl.display.Attach(draw9.RefNone); err != nil {
				log.Fatalf("display: failed to attach: %v", err)
			}
			iv.Attach(dctl.display.Image.Bounds())
			iv.paint(dctl)
		}
	}
}

func (iv *IconsView) paint(dctl *DisplayControl) {
	dctl.showWaitingAndCall(func() {
		from, to := iv.offset.Visible()
		images := slices.Collect(Get(iv.iconsCache, from, to))
		paintIcons(dctl, iv.offset.grid, images)
	})
}

// moveUpToNextPageWithMarked moves up to the next page with a marked icon.
func (iv *IconsView) moveUpToNextPageWithMarked() {
	i, _ := slices.BinarySearch(iv.pagesWithMarked, iv.offset.CurrentPage())
	if i > 0 {
		iv.offset.GotoPage(iv.pagesWithMarked[i-1])
	}
}

// moveDownToNextPageWithMarked moves down to the next page with a marked icon.
func (iv *IconsView) moveDownToNextPageWithMarked() {
	i, found := slices.BinarySearch(iv.pagesWithMarked, iv.offset.CurrentPage())
	if found {
		i++
	}
	if i < len(iv.pagesWithMarked) {
		iv.offset.GotoPage(iv.pagesWithMarked[i])
	}
}

func (iv *IconsView) resetPagesWithMarked() {
	iv.pagesWithMarked = iv.pagesWithMarked[0:0]
	for i, icon := range iv.icons {
		if icon.marked {
			if p := iv.offset.PageOfItem(i); !slices.Contains(iv.pagesWithMarked, p) {
				iv.pagesWithMarked = append(iv.pagesWithMarked, p)
			}
		}
	}
}

func (iv *IconsView) toggleMarked(i int) {
	if icon, ok := iv.iconsCache.At(i); ok {
		icon.ToggleMarked()
	}
	iv.resetPagesWithMarked()
}

func (iv *IconsView) collectMarkedIcons() []*Icon {
	var icons []*Icon
	for _, icon := range iv.icons {
		if icon.marked {
			icons = append(icons, icon)
		}
	}
	return icons
}
