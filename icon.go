package main

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"log"
	"net/http"
	"os"
	"strings"

	draw9 "9fans.net/go/draw"
	"github.com/xor-gate/goexif2/exif"
	"github.com/xor-gate/goexif2/tiff"
	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

var (
	// fastScaler is used to scale small icons.
	fastScaler xdraw.Scaler = xdraw.BiLinear
	// bestScaler is used to scale large images.
	bestScaler xdraw.Scaler = xdraw.CatmullRom
)

// Displayer returns the display version of the image.
type Displayer func(image.Image) (*draw9.Image, error)

// Icon is an image for viewing.
type Icon struct {
	path   string // path of the image file
	marked bool   // true if marked by the user
}

// IconImage hold the contents of an icon.
type IconImage struct {
	*Icon                  // the origin of the image
	data      []byte       // the image contents from file
	thumb     *draw9.Image // thumbnail for display
	displayer Displayer    // function to compute the display for the image
	exifInfo  string       // a summary of the EXIF data if present
}

var (
	errNotSupportedFormat = errors.New("not supported format")
)

// NewIcon returns a new Icon for path.
func NewIcon(path string) *Icon {
	return &Icon{path: path}
}

// NewIconImage returns a new instance for the contents of icons.
func (i *Icon) NewIconImage(displayer Displayer) *IconImage {
	return &IconImage{Icon: i, displayer: displayer}
}

// ToggleMarked marks/unmarks the icon
func (i *Icon) ToggleMarked() {
	i.marked = !i.marked
}

func (i *IconImage) ForDisplay() (*draw9.Image, error) {
	if err := i.Load(); err != nil {
		return nil, err
	}
	return i.thumb, nil
}

// Loads load the image from the file.
func (i *IconImage) Load() error {
	if i.data == nil {
		data, err := os.ReadFile(i.path)
		if err != nil {
			return fmt.Errorf("load: %w", err)
		}

		switch ct := http.DetectContentType(data); ct {
		case "image/gif", "image/jpeg", "image/png", "image/webp":
			// supported format
		default:
			return fmt.Errorf("load: cannot handle %s: %w", ct, errNotSupportedFormat)
		}

		i.exifInfo = getExifInfo(bytes.NewReader(data))
		i.data = data
	}

	if i.thumb == nil {
		img, _, err := image.Decode(bytes.NewBuffer(i.data))
		if err != nil {
			return fmt.Errorf("load: decode image: %w", err)
		}
		thumb, err := i.displayer(img)
		if err != nil {
			return fmt.Errorf("load: display image: %w", err)
		}
		i.thumb = thumb
	}

	return nil
}

// Unload frees the image data. To use it again, call Load first.
func (i *IconImage) Unload() {
	if i.data == nil {
		return
	}

	i.data = nil
	if i.thumb != nil {
		if err := i.thumb.Free(); err != nil {
			log.Printf("unload: failed to free thumbnail %s: %v", i.path, err)
		}
		i.thumb = nil
	}
}

// FitFast fits img in r using a fast algorithm and an acceptable result.
func FitFast(disp *draw9.Display, img image.Image, r image.Rectangle) (*draw9.Image, error) {
	dr := bestFit(r, img.Bounds())
	dimg := image.NewRGBA(dr)
	fastScaler.Scale(dimg, dr, img, img.Bounds(), xdraw.Src, nil)
	t, err := disp.ReadImage(toPlan9Bitmap(dimg))
	if err != nil {
		return nil, err
	}
	return t, nil
}

// FitBest fits img in r produces the best result but it is slow.
func FitBest(disp *draw9.Display, img image.Image, r image.Rectangle) (*draw9.Image, error) {
	dr := bestFit(r, img.Bounds())
	dimg := image.NewRGBA(dr)
	bestScaler.Scale(dimg, dr, img, img.Bounds(), xdraw.Src, nil)
	t, err := disp.ReadImage(toPlan9Bitmap(dimg))
	if err != nil {
		return nil, err
	}
	return t, nil
}

// toPlan9Bitmap converts an image to the plan9 format for display.
func toPlan9Bitmap(img *image.RGBA) *bytes.Buffer {
	n := 60 + img.Bounds().Dx()*img.Bounds().Dy()*4
	b := bytes.NewBuffer(make([]byte, 0, n))
	fmt.Fprintf(b, "%11s %11d %11d %11d %11d ",
		"r8g8b8a8", 0, 0, img.Bounds().Dx(), img.Bounds().Dy())
	for data := img.Pix; len(data) > 0; data = data[4:] {
		b.WriteByte(data[3])
		b.WriteByte(data[2])
		b.WriteByte(data[1])
		b.WriteByte(data[0])
	}
	return b
}

// getExifInfo returns an online human readable string of the exif data.
func getExifInfo(r tiff.ReadAtReaderSeeker) string {
	ex, err := exif.Decode(r)
	if err != nil {
		return ""
	}

	asString := func(t *tiff.Tag) string {
		return t.String()
	}

	asRatFloat := func(t *tiff.Tag) string {
		f, _ := t.Rat(0)
		return f.FloatString(2)
	}

	labels := []struct {
		pat     string
		name    exif.FieldName
		printer func(*tiff.Tag) string
	}{
		{"Date: %s", exif.DateTimeOriginal, asString},
		{"Model: %s", exif.Model, asString},
		{"f/%s", exif.FNumber, asRatFloat},
		{"Exp: %s", exif.ExposureTime, asRatFloat},
		{"ISO: %s", exif.ISOSpeedRatings, asString},
		{"Shutter: %s", exif.ShutterSpeedValue, asRatFloat},
	}

	nwrites := 0
	var b strings.Builder
	b.WriteString("Exif: ")
	for _, label := range labels {
		if tag, err := ex.Get(label.name); err == nil {
			b.WriteString(fmt.Sprintf(label.pat, label.printer(tag)))
			b.WriteString(" ")
			nwrites++
		}
	}
	if nwrites > 0 {
		return b.String()
	}
	return ""
}

// NewIconImages is the slice version of Icon.NewIconImage.
func NewIconImages(icons []*Icon, displayer Displayer) []*IconImage {
	var images []*IconImage
	for _, icon := range icons {
		images = append(images, icon.NewIconImage(displayer))
	}
	return images
}
