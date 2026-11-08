package rendering

import (
	"testing"

	"gonetsurf/rendering/interfaces"
	"gonetsurf/rendering/software"
)

type Surface = software.Surface
type Compositor = software.Compositor
type TileGrid = software.TileGrid
type Damage = software.Damage

func NewSurface(width, height int) *Surface { return software.NewSurface(width, height) }

func NewCompositor(width, height int) *Compositor {
	return software.NewCompositor(width, height)
}

func NewTileGrid(width, height, tileSize int) *TileGrid {
	return software.NewTileGrid(width, height, tileSize)
}

func asSoftwareSurface(t *testing.T, surface interfaces.Surface) *Surface {
	t.Helper()
	typed, ok := surface.(*Surface)
	if !ok {
		t.Fatalf("renderer returned %T, want *software.Surface", surface)
	}
	return typed
}
