package rendering

import (
	"testing"

	"gonetsurf/rendering/software"
)

func FuzzSVG(f *testing.F) {
	for _, seed := range []string{
		`<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10"><rect width="10" height="10"/></svg>`,
		`<svg viewBox="0 0 10 10"><path d="M0 0 L10 10 Z"/></svg>`,
		`<svg><circle cx="5" cy="5" r="4" fill="url(#g)"/><defs><linearGradient id="g"><stop offset="0" stop-color="red"/><stop offset="1" stop-color="blue"/></linearGradient></defs></svg>`,
		`<svg><script>alert(1)</script><foreignObject><div/></foreignObject></svg>`,
	} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = software.DecodeSVG(data, software.SVGOptions{MaxPixels: 1 << 18, MaxDimension: 1024, MaxNodes: 4096, MaxDepth: 64})
	})
}
