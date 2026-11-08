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
	mutex    sync.Mutex
	entries  map[string]*imageCacheEntry
	callback func(string)
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
	cache.mutex.Lock()
	callback := cache.callback
	cache.mutex.Unlock()
	if callback != nil {
		callback(key)
	}
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

// SetLoadCallback installs a function invoked after an image load finishes.
// The callback runs on the loading goroutine and must not mutate layout state
// directly.
func (cache *ImageCache) SetLoadCallback(callback func(string)) {
	if cache == nil {
		return
	}
	cache.mutex.Lock()
	cache.callback = callback
	cache.mutex.Unlock()
}

func (cache *ImageCache) peek(key string) (*image.NRGBA, bool) {
	cache.mutex.Lock()
	entry, ok := cache.entries[key]
	cache.mutex.Unlock()
	if !ok {
		return nil, false
	}
	select {
	case <-entry.done:
		if entry.err != nil || entry.image == nil {
			return nil, false
		}
		return entry.image, true
	default:
		return nil, false
	}
}

// PeekImage returns a decoded image without blocking. The second result is
// false while the image is still loading or after a failed load.
func (provider CachedImageProvider) PeekImage(resource string) (image.Image, bool) {
	if provider.Provider == nil {
		return nil, false
	}
	cache := provider.Cache
	if cache == nil {
		return nil, false
	}
	imageValue, ok := cache.peek(provider.cacheKey(resource))
	if !ok {
		return nil, false
	}
	return imageValue, true
}

// SetLoadCallback installs an image load callback on the shared cache.
func (provider CachedImageProvider) SetLoadCallback(callback func(string)) {
	if provider.Cache != nil {
		provider.Cache.SetLoadCallback(callback)
	}
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
