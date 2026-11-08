package shells

import (
	"testing"

	"gonetsurf/rendering/interfaces"
	"gonetsurf/rendering/software"
)

func asSoftwareSurface(t *testing.T, surface interfaces.Surface) *software.Surface {
	t.Helper()
	typed, ok := surface.(*software.Surface)
	if !ok {
		t.Fatalf("renderer returned %T, want *software.Surface", surface)
	}
	return typed
}
