package main

import (
	"flag"
	"fmt"
	"image"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"runtime/pprof"
	"slices"
	"strconv"
	"strings"

	draw9 "9fans.net/go/draw"
	"9fans.net/go/plan9"
	"9fans.net/go/plan9/client"
	"9fans.net/go/plumb"
	xdraw "golang.org/x/image/draw"
)

const (
	progName = "iview"

	darkgrey = draw9.Color(uint32(0x666666FF))
	yellow   = draw9.Color(uint32(0xFFFF00FF))

	upArrowKey      = 61454
	downArrowKey    = 128
	leftArrowKey    = 61457
	rightArrowKey   = 61458
	scrollWheelUp   = 8
	scrollWheelDown = 16
	escKey          = 27
)

var (
	windowSizeFlag = flag.String("w", "1300x1000", "set window size")
	iconSizeFlag   = flag.String("i", "320x240", "set icon size")
	outputMarked   = flag.Bool("o", false, "output the paths of marked images")
	startSingle    = flag.Bool("s", false, "start with the single view")
	silent         = flag.Bool("q", false, "silent mode, do not log anything")
	verbose        = flag.Bool("v", false, "verbose mode, log statistics for cache")
	fast           = flag.Bool("f", false, "choose fast over best algorithms for scaling")
	pageSize       = flag.Int("p", 0, "set page size. Default is 1 grid page")
	setMemoryLimit = flag.Bool("m", false, "run with 1G soft memory limit. Overrides GOMEMLIMIT")
)

var (
	enableProfiler = flag.Bool("profile", false, "run with the profiler enabled")
	cpuprofile     = flag.String("cpuprofile", "cpu.prof", "write cpu profile to `file`")
	memprofile     = flag.String("memprofile", "mem.prof", "write memory profile to `file`")
)

var (
	windowSize      image.Point
	iconSize        image.Point
	padding         = 4
	acceptedFormats = []string{".gif", ".jpg", ".jpeg", ".png", ".webp"}

	plumber *client.Fid
)

type DisplayControl struct {
	display     *draw9.Display
	errch       chan error
	mctl        *draw9.Mousectl
	kctl        *draw9.Keyboardctl
	bgColor     *draw9.Image
	borderColor *draw9.Image
	fontColor   *draw9.Image
}

func usage() {
	fmt.Fprintf(os.Stderr, `usage: %s [-f|-o|-q|-v|-s|-m] [file|dir]..

%s is an image viewer.

Flags:
`, progName, progName)
	flag.PrintDefaults()
	os.Exit(2)
}

func main() {
	log.SetPrefix("")
	log.SetFlags(0)
	flag.Usage = usage
	flag.Parse()

	if *enableProfiler {
		f, err := os.Create(*cpuprofile)
		if err != nil {
			log.Fatal("could not create CPU profile: ", err)
		}
		defer f.Close()
		if err := pprof.StartCPUProfile(f); err != nil {
			log.Fatal("could not start CPU profile: ", err)
		}
		defer pprof.StopCPUProfile()
	}

	var ok bool
	windowSize, ok = stringToPoint(*windowSizeFlag)
	if !ok {
		log.Fatalf("cannot compute window size from %s", *windowSizeFlag)
	}

	iconSize, ok = stringToPoint(*iconSizeFlag)
	if !ok {
		log.Fatalf("cannot compute icon size from %s", *iconSizeFlag)
	}

	if *setMemoryLimit {
		debug.SetMemoryLimit(1 * 1024 * 1024) // or GOMEMLIMIT=1GiB
	}

	if *silent {
		log.SetOutput(io.Discard)
	}

	if *fast {
		fastScaler = xdraw.NearestNeighbor
		bestScaler = xdraw.BiLinear
	}

	var icons []*Icon
	for _, p := range flag.Args() {
		icons = append(icons, addImagesOfPath(p)...)
	}
	if len(icons) == 0 {
		os.Exit(0)
	}

	connectToPlumber()
	dctl := connectToDisplay(windowSize)
	dctl.cls()

	grid := NewGrid(dctl.display.Image.Bounds(), iconSize, padding)
	iv := NewIconsView(icons, grid, *pageSize)
	iv.Connect(dctl)

	var views []View
	views = append(views, iv)
	if *startSingle {
		sv := NewSingleView(icons, 0, iv.offset.grid.area)
		sv.Connect(dctl)
		views = append(views, sv)
	}
	for len(views) > 0 {
		v := views[len(views)-1]
		v.Attach(dctl.display.Image.Bounds())
		if nv := v.Handle(); nv != nil {
			nv.Connect(dctl)
			views = append(views, nv)
		} else {
			views = views[0 : len(views)-1]
			if len(views) > 0 {
				syncViewsOnExit(v, views[len(views)-1])
			}
		}
	}

	if *enableProfiler {
		f, err := os.Create(*memprofile)
		if err != nil {
			log.Fatal("could not create memory profile: ", err)
		}
		defer f.Close()
		runtime.GC()
		if err := pprof.WriteHeapProfile(f); err != nil {
			log.Fatal("could not write memory profile: ", err)
		}
	}

	if *outputMarked {
		for _, icon := range icons {
			if icon.marked {
				fmt.Println(icon.path)
			}
		}
	}
}

// syncViewsOnExit is an ugly hack to sync the position of
// the singleview with the page of iconsview.
// It is simpler than augment the View interface with some callbacks.
func syncViewsOnExit(viewExited, viewToGo View) {
	if sv, ok1 := viewExited.(*SingleView); ok1 {
		if iv, ok2 := viewToGo.(*IconsView); ok2 {
			iv.offset.GotoPage(iv.offset.PageOfItem(sv.at))
		}
	}
}

// isImageFile checks the file suffix to check if it is an image.
func isImageFile(name string) bool {
	return slices.Contains(acceptedFormats, strings.ToLower(filepath.Ext(name)))
}

// addImagesOfPath adds the image at path, descending it if a directory.
func addImagesOfPath(name string) []*Icon {
	info, err := os.Stat(name)
	if err != nil {
		log.Printf("addImagesOfPath: cannot stat file: %v", err)
		return nil
	}
	if info.IsDir() {
		return scanForImages(name)
	}
	if !info.Mode().IsRegular() {
		log.Printf("addImagesOfPath: ignoring special file %s", name)
		return nil
	}
	if !isImageFile(name) {
		return nil
	}
	return []*Icon{NewIcon(name)}
}

// scanForImages walks dir and adds the images found.
func scanForImages(dir string) []*Icon {
	var icons []*Icon

	walkFn := func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !d.Type().IsRegular() {
			log.Printf("scanForImages: ignoring special file %s", path)
			return nil
		}
		if !isImageFile(path) {
			return nil
		}
		icons = append(icons, NewIcon(path))
		return nil
	}

	if err := filepath.WalkDir(dir, walkFn); err != nil {
		log.Printf("scanForImages: %s: %v", dir, err)
	}

	return icons
}

func connectToDisplay(dims image.Point) *DisplayControl {
	errch := make(chan error)
	disp, err := draw9.Init(errch, "", progName, fmt.Sprintf("%dx%d", dims.X, dims.Y))
	if err != nil {
		log.Fatalf("display: cannot connect: %v", err)
	}
	kctl := disp.InitKeyboard()
	mctl := disp.InitMouse()

	return &DisplayControl{
		display:     disp,
		errch:       errch,
		mctl:        mctl,
		kctl:        kctl,
		bgColor:     disp.AllocImageMix(darkgrey, darkgrey),
		borderColor: disp.AllocImageMix(darkgrey, yellow),
		fontColor:   disp.AllocImageMix(darkgrey, yellow),
	}
}

// showWaitingAndCall changes the cursor to the waiting one and executes fn
func (dctl *DisplayControl) showWaitingAndCall(fn func()) {
	if err := dctl.display.SwitchCursor(lockarrow); err != nil {
		log.Printf("failed to switch cursor: %v", err)
	}
	fn()
	if err := dctl.display.SwitchCursor(nil); err != nil {
		log.Printf("failed to switch cursor: %v", err)
	}
}

func (dctl *DisplayControl) cls() {
	dctl.display.Image.Draw(dctl.display.Image.Bounds(), dctl.bgColor, nil, image.Point{})
	dctl.display.Flush()
}

func connectToPlumber() {
	var err error
	plumber, err = plumb.Open("send", plan9.OWRITE|plan9.OCEXEC)
	if err != nil {
		log.Printf("plumber not available: %v", err)
	}
}

func plumbImage(s string) {
	if plumber == nil {
		log.Printf("plumber not available")
		return
	}

	m := plumb.Message{
		Src:  progName,
		Dir:  filepath.Dir(s),
		Type: "text",
		Data: []byte(s),
	}
	if err := m.Send(plumber); err != nil {
		log.Printf("plumber: %v", err)
	}
}

func stringToPoint(s string) (image.Point, bool) {
	fields := strings.Split(s, "x")
	if len(fields) != 2 {
		return image.Point{}, false
	}
	x, err := strconv.Atoi(fields[0])
	if err != nil {
		return image.Point{}, false
	}
	y, err := strconv.Atoi(fields[1])
	if err != nil {
		return image.Point{}, false
	}
	return image.Pt(x, y), true
}
