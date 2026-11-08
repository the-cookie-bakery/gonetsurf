package rendering

import (
	"fmt"
	"image"
	"log"
	"os"
	"sync"

	"gonetsurf/urls"
)

type imageCacheEntry struct {
	image *image.NRGBA
	err   error
	done  chan struct{}
}

// ImageCache is a concurrency-safe cache of decoded images. It is shared
// across frames so repeated paints do not refetch or re-decode subresources.
type ImageCache struct {
	mutex   sync.Mutex
	entries map[string]*imageCacheEntry
}

// NewImageCache returns an empty image cache.
func NewImageCache() *ImageCache {
	return &ImageCache{entries: make(map[string]*imageCacheEntry)}
}

func (cache *ImageCache) load(key string, loader func() (image.Image, error)) (*image.NRGBA, error) {
	cache.mutex.Lock()
	if entry, ok := cache.entries[key]; ok {
		cache.mutex.Unlock()
		<-entry.done
		return entry.image, entry.err
	}
	entry := &imageCacheEntry{done: make(chan struct{})}
	cache.entries[key] = entry
	cache.mutex.Unlock()

	loaded, err := loader()
	if err == nil && loaded != nil {
		entry.image = toNRGBA(loaded)
	}
	if err == nil && loaded == nil {
		err = fmt.Errorf("rendering: image loader returned no image")
	}
	entry.err = err
	close(entry.done)
	if err != nil {
		cache.mutex.Lock()
		delete(cache.entries, key)
		cache.mutex.Unlock()
	}
	return entry.image, entry.err
}

// CachedImageProvider wraps an ImageProvider with a shared decoded-image cache.
type CachedImageProvider struct {
	Provider ImageProvider
	Cache    *ImageCache
	Base     *urls.URL
}

// LoadImage returns a cached image or delegates to the wrapped provider.
func (provider CachedImageProvider) LoadImage(resource string) (image.Image, error) {
	if provider.Provider == nil {
		return nil, fmt.Errorf("rendering: no image provider configured")
	}
	cache := provider.Cache
	if cache == nil {
		cache = NewImageCache()
	}
	loaded, err := cache.load(provider.cacheKey(resource), func() (image.Image, error) {
		return provider.Provider.LoadImage(resource)
	})
	if err != nil {
		if os.Getenv("GONETSURF_DEBUG") != "" {
			log.Printf("image: load failed %s: %v", resource, err)
		}
		return nil, err
	}
	return loaded, nil
}

// Prefetch loads resources concurrently into the cache. It is safe to call
// while the painter is running; in-flight loads are shared.
func (provider CachedImageProvider) Prefetch(resources []string) {
	const workers = 10
	work := make(chan string)
	var waitGroup sync.WaitGroup
	for index := 0; index < workers; index++ {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			for resource := range work {
				_, _ = provider.LoadImage(resource)
			}
		}()
	}
	for _, resource := range resources {
		work <- resource
	}
	close(work)
	waitGroup.Wait()
}

func (provider CachedImageProvider) cacheKey(resource string) string {
	address, ok := parseCSSURL(resource)
	if !ok {
		return resource
	}
	if provider.Base == nil {
		return address
	}
	if target, err := urls.ParseReference(provider.Base, address); err == nil {
		return target.String()
	}
	return address
}
