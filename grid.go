package main

import "image"

// Grid overlays on area a maximal MxN grid of icons. The dimensions are calculated
// from the iconSize and the padding.
type Grid struct {
	area     image.Rectangle
	iconSize image.Point
	padding  int
}

// Offset is used with a grid and an implicit slice to track which items should be displayed.
// A MxN grid will display items [pos, pos + M*N) of the slice.
type Offset struct {
	grid  *Grid
	pos   int
	limit int
}

// newGrid returns a new grid.
func NewGrid(area image.Rectangle, iconSize image.Point, padding int) *Grid {
	return &Grid{
		area:     area,
		iconSize: iconSize,
		padding:  padding,
	}
}

// attach should be called when the grid area changes.
func (g *Grid) Attach(r image.Rectangle) {
	g.area = r
}

// dimensions return the grid dimensions, rows x columns.
func (g *Grid) Dimensions() (rows int, cols int) {
	rows = (g.area.Dy() - g.padding) / (g.iconSize.Y + g.padding)
	cols = (g.area.Dx() - g.padding) / (g.iconSize.X + g.padding)
	return
}

// Area returns the icon area of the grid, rows * columns.
func (g *Grid) Area() int {
	rows, cols := g.Dimensions()
	return rows * cols
}

// gridCoords translates the area coordinates to the grid coordinates.
func (g *Grid) GridCoords(at image.Point) (x int, y int, inside bool) {
	h := g.PaintableArea()
	inside = at.In(h)
	if !inside {
		return
	}
	x = (at.X - h.Min.X) / g.iconSize.X
	y = (at.Y - h.Min.Y) / g.iconSize.Y
	return
}

// PaintableArea is the area of the grid which contains icons.
// Only full icons are displayed and there maybe empty space at the edges of grid.area
func (g *Grid) PaintableArea() image.Rectangle {
	rows, cols := g.Dimensions()
	ir := image.Rect(0, 0,
		cols*(g.iconSize.X+g.padding), rows*(g.iconSize.Y+g.padding))
	return center(g.area, ir)
}

// NewOffset returns a new offset with limit and grid.
func NewOffset(grid *Grid, limit int) *Offset {
	return &Offset{grid: grid, limit: limit}
}

// Visible returns the visible items for the current grid page.
func (o *Offset) Visible() (int, int) {
	return o.pos, min(o.limit, o.pos+o.grid.Area())
}

// CurrentPage return the current page.
func (o *Offset) CurrentPage() int {
	return o.PageOfItem(o.pos)
}

// PageOfItem returns the screen page of the item.
func (o *Offset) PageOfItem(i int) int {
	if 0 <= i && i < o.limit {
		return i / o.grid.Area()
	}
	return -1
}

// MoveUpRow scrolls the page one grid row up.
func (o *Offset) MoveUpRow() {
	_, cols := o.grid.Dimensions()
	o.pos = max(0, o.pos-cols)
}

// MoveDownRow scrolls the page one grid row down.
func (o *Offset) MoveDownRow() {
	rows, cols := o.grid.Dimensions()
	// add cols as an offset so that it displays
	// an empty row at the end of the icons
	o.pos = min(o.pos+cols, max(0, o.limit-(rows*cols)+cols))
}

// GotoPage moves view to page.
func (o *Offset) GotoPage(page int) {
	numPages := intCeil(o.limit, o.grid.Area())
	if 0 <= page && page < numPages {
		o.pos = page * o.grid.Area()
	}
}

// At computes the offset under the point.
func (o *Offset) At(p image.Point) (int, bool) {
	x, y, inside := o.grid.GridCoords(p)
	if !inside {
		return -1, false
	}

	_, cols := o.grid.Dimensions()
	position := o.pos + y*cols + x
	return position, position < o.limit
}
