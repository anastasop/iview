package main

import "image"

// View receives input from mouse, keyboard and paints on screen.
// They are designed to stack up, so a view may return another
// view to take its place.
type View interface {
	// Connect connects the view with the display. Used for initialization.
	Connect(dctl *DisplayControl)

	// Handle is like main for a View.
	// Returns an optional View to replace it. This allows push/pop views and sharing.
	Handle() View

	// attach should be called to reattach to display after a resize.
	Attach(image.Rectangle)

	// free releases the view resources, like cached images.
	Free()
}
