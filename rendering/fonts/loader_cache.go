package fonts

import "sync"

type loaderEntry struct {
	data []byte
	err  error
	done chan struct{}
}

// LoaderCache memoizes raw font bytes and shares in-flight loads.
type LoaderCache struct {
	mutex   sync.Mutex
	entries map[string]*loaderEntry
}

// NewLoaderCache returns an empty font loader cache.
func NewLoaderCache() *LoaderCache {
	return &LoaderCache{entries: make(map[string]*loaderEntry)}
}

// Load returns cached font bytes or delegates to loader.
func (cache *LoaderCache) Load(resource string, loader Loader) ([]byte, error) {
	if cache == nil {
		return loader(resource)
	}
	cache.mutex.Lock()
	if entry, ok := cache.entries[resource]; ok {
		cache.mutex.Unlock()
		<-entry.done
		return entry.data, entry.err
	}
	entry := &loaderEntry{done: make(chan struct{})}
	cache.entries[resource] = entry
	cache.mutex.Unlock()
	data, err := loader(resource)
	entry.data = data
	entry.err = err
	close(entry.done)
	if err != nil {
		cache.mutex.Lock()
		delete(cache.entries, resource)
		cache.mutex.Unlock()
	}
	return data, err
}

// Loader returns a Loader that consults the cache.
func (cache *LoaderCache) Loader(loader Loader) Loader {
	return func(resource string) ([]byte, error) {
		return cache.Load(resource, loader)
	}
}

// Prefetch loads font resources concurrently.
func (cache *LoaderCache) Prefetch(resources []string, loader Loader) {
	const workers = 6
	work := make(chan string)
	var waitGroup sync.WaitGroup
	for index := 0; index < workers; index++ {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			for resource := range work {
				_, _ = cache.Load(resource, loader)
			}
		}()
	}
	for _, resource := range resources {
		work <- resource
	}
	close(work)
	waitGroup.Wait()
}
