package main

import (
	"fmt"
	"iter"
	"log"
	"slices"
	"sync"
	"time"
)

// CachedItem is anything that can be lazily loaded and unloaded.
type CachedItem interface {
	// Loads loads the item and prepares it for use.
	Load() error
	// Unload releases the resources of the item. To use it again,
	// the caller must call Load.
	Unload()
}

// CachedSlice is a slice of CachedItems. It maintains a cache of loaded items.
type CachedSlice[E CachedItem] interface {
	// At returns the ith item and ensures it is loaded. It also returns a bool
	// saying whether the slice contains the item.
	At(i int) (E, bool)
	// Len returns the length of the slice.
	Len() int
	// Free clears the cache and unloads all items. The cache cannot be reused after this.
	Free()
}

// Get returns the items in [from, to) as an iterator.
func Get[E CachedItem](c CachedSlice[E], from, to int) iter.Seq[E] {
	return func(yield func(E) bool) {
		for ; from < to; from++ {
			i, ok := c.At(from)
			if !ok || !yield(i) {
				return
			}
		}
	}
}

// CachedSlicePaged is a CachedSlice in which the slice is split into
// pages and the most frequently used ones are cached. It tries
// to be a bit proactive and fetch some pages before use.
type CachedSlicePaged[E CachedItem] struct {
	name     string
	items    []E
	pageSize int
	fetchC   chan<- pageRequest
}

// NewCachedSlicePaged returns a CachedSlicePaged for the items and sets the page size.
// It starts a goroutine to fetch pages before use. Caller must call Free to release it
// after use.
func NewCachedSlicePaged[E CachedItem](name string, items []E, pageSize int) *CachedSlicePaged[E] {
	if *verbose {
		log.Printf("cache %s(%d/%d): %d pages",
			name, len(items), pageSize, intCeil(len(items), pageSize))
	}
	c := new(CachedSlicePaged[E])
	c.name = name
	c.items = items
	c.pageSize = pageSize
	c.startPreFetcher()
	return c
}

func (c *CachedSlicePaged[E]) At(pos int) (E, bool) {
	if pos >= len(c.items) {
		var z E
		return z, false
	}
	page := pos / c.pageSize
	c.fetchPagesLater(page-1, page+1)
	c.fetchPageNow(page)
	return c.items[pos], true
}

func (c *CachedSlicePaged[E]) Len() int {
	return len(c.items)
}

func (c *CachedSlicePaged[E]) Free() {
	c.stopPreFetcher()
	for i := 0; i < len(c.items); i++ {
		go func(j int) {
			c.items[j].Unload()
		}(i)
	}
}

// numPages returns the total number of pages.
func (c *CachedSlicePaged[E]) numPages() int {
	return intCeil(len(c.items), c.pageSize)
}

// pageRequest is a request to the page fetcher for a page.
type pageRequest struct {
	// page is the page number to load.
	page int
	// done is an optional channel to notify after load. Should be buffered.
	done chan int
}

// fetchPageNow requests a page and waits until is loaded.
func (c *CachedSlicePaged[E]) fetchPageNow(p int) {
	if 0 <= p && p < c.numPages() {
		r := pageRequest{p, make(chan int, 1)}
		c.fetchC <- r
		<-r.done
	}
}

// fetchPagesLater requests some pages and returns. The pages are loaded in the background.
func (c *CachedSlicePaged[E]) fetchPagesLater(pages ...int) {
	for _, p := range pages {
		if 0 <= p && p < c.numPages() {
			c.fetchC <- pageRequest{p, nil}
		}
	}
}

// startPreFetcher launches the goroutine that (pre)fetches pages and maintains the cache.
// All requests for pages should be handled with messages to c.fetchC
func (c *CachedSlicePaged[E]) startPreFetcher() {
	in := make(chan pageRequest)
	c.fetchC = in
	go func() {
		var cache pageCache
		var inflight loader

		ready := make(chan int)
		for {
			select {
			case req, ok := <-in:
				if !ok {
					return
				}
				if cache.contains(req.page) {
					if req.done != nil {
						req.done <- req.page
					}
				} else if inflight.track(req) {
					go func(p int) {
						if *verbose {
							defer func(start time.Time) {
								log.Printf("cache %s(%d/%d): load page %d time %v",
									c.name, len(c.items), c.pageSize, p, time.Since(start))
							}(time.Now())
						}
						c.loadPage(p)
						ready <- p
					}(req.page)
				}
			case page := <-ready:
				if !inflight.isActive(page) {
					panic(fmt.Sprintf("cache: ready page %d not inprogress", page))
				}
				if ep, evicted := cache.add(page); evicted {
					go func(p int) {
						if *verbose {
							log.Printf("cache %s(%d/%d): evicted page %d",
								c.name, len(c.items), c.pageSize, p)
						}
						c.unloadPage(p)
					}(ep)
				}
				if *verbose {
					log.Printf("cache %s(%d/%d): pages %v",
						c.name, len(c.items), c.pageSize, cache.pages)
				}
				inflight.done(page)
			}
		}
	}()
}

// stopPreFetcher stops the fetcher goroutine. After this the cache is unusable.
func (c *CachedSlicePaged[E]) stopPreFetcher() {
	if c.fetchC != nil {
		close(c.fetchC)
	}
	c.fetchC = nil
}

// loadPage loads all the items of the page.
func (c *CachedSlicePaged[E]) loadPage(p int) {
	c.mapPageItems(p, func(item E) { item.Load() })
}

// unloadPage unloads all the items of the page.
func (c *CachedSlicePaged[E]) unloadPage(p int) {
	c.mapPageItems(p, func(item E) { item.Unload() })
}

// mapPageItems processes all the items of a page in parallel.
func (c *CachedSlicePaged[E]) mapPageItems(p int, fn func(item E)) {
	begin := p * c.pageSize
	end := min(len(c.items), begin+c.pageSize)
	var wg sync.WaitGroup
	wg.Add(end - begin)
	for i := begin; i < end; i++ {
		go func(j int) {
			defer wg.Done()
			fn(c.items[j])
		}(i)
	}
	wg.Wait()
}

// pageCache is cache storage for pages.
type pageCache struct {
	pages []int
}

// contains returns whether the page is in the cache.
func (pc *pageCache) contains(page int) bool {
	return slices.Contains(pc.pages, page)
}

// add adds the page in the cache. If the cache is full, it evicts
// the least frequently used page and returns it. The bool tells
// if a page was evicted.
func (pc *pageCache) add(page int) (int, bool) {
	if pc.contains(page) {
		return 0, false
	}

	const cacheSize = 5
	if len(pc.pages) < cacheSize {
		pc.pages = append(pc.pages, page)
		return 0, false
	}

	pc.pages = append(pc.pages, page)
	slices.Sort(pc.pages)
	var evicted int
	if i := slices.Index(pc.pages, page); i >= cacheSize-i-1 {
		evicted = pc.pages[0]
		copy(pc.pages, pc.pages[1:])
	} else {
		evicted = pc.pages[cacheSize-1]
	}
	pc.pages = pc.pages[0:cacheSize]
	return evicted, true
}

// inProgress is an active page request.
type inProgress struct {
	p     int        // the page number
	reply []chan int // channels to notify after loading
}

// loader tracks the active page requests.
type loader struct {
	loading []inProgress
}

// isActive returns where a request for page is already in progress.
func (l *loader) isActive(page int) bool {
	return slices.ContainsFunc(l.loading, func(this inProgress) bool {
		return this.p == page
	})
}

// track tracks a request for a page. Returns whether this is
// a request for a new page.
func (l *loader) track(req pageRequest) bool {
	// append c to s only if c is not nil
	appendNotNil := func(s []chan int, c chan int) []chan int {
		if c != nil {
			s = append(s, c)
		}
		return s
	}

	i := slices.IndexFunc(l.loading, func(this inProgress) bool {
		return this.p == req.page
	})
	if i != -1 {
		l.loading[i].reply = appendNotNil(l.loading[i].reply, req.done)
		return false
	}

	l.loading = append(l.loading, inProgress{req.page, appendNotNil(nil, req.done)})
	return true
}

// done removes tracking for the page. It notifies requesters.
func (l *loader) done(page int) {
	i := slices.IndexFunc(l.loading, func(this inProgress) bool {
		return this.p == page
	})
	if i >= 0 {
		for _, c := range l.loading[i].reply {
			c <- page
		}
		l.loading[i] = l.loading[len(l.loading)-1]
		l.loading = l.loading[0 : len(l.loading)-1]
	}
}
