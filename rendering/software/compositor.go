package software

import (
	"image"
	"sort"
)

// Damage accumulates dirty rectangles for a frame.
type Damage struct {
	rects []image.Rectangle
}

// Add marks rect as damaged, merging overlapping or adjacent rectangles.
func (damage *Damage) Add(rect image.Rectangle) {
	if damage == nil || rect.Empty() {
		return
	}
	damage.rects = append(damage.rects, rect)
	damage.merge()
}

// AddAll marks the whole surface as damaged.
func (damage *Damage) AddAll(width, height int) {
	damage.Add(image.Rect(0, 0, width, height))
}

// Empty reports whether no damage is recorded.
func (damage *Damage) Empty() bool {
	return damage == nil || len(damage.rects) == 0
}

// Rects returns the merged damage rectangles.
func (damage *Damage) Rects() []image.Rectangle {
	if damage == nil {
		return nil
	}
	return append([]image.Rectangle(nil), damage.rects...)
}

// Bounds returns the union of all damage rectangles.
func (damage *Damage) Bounds() image.Rectangle {
	if damage == nil || len(damage.rects) == 0 {
		return image.Rectangle{}
	}
	result := damage.rects[0]
	for _, rect := range damage.rects[1:] {
		result = result.Union(rect)
	}
	return result
}

// Clear resets the damage list.
func (damage *Damage) Clear() {
	damage.rects = nil
}

func (damage *Damage) merge() {
	for {
		merged := false
		for first := 0; first < len(damage.rects) && !merged; first++ {
			for second := first + 1; second < len(damage.rects); second++ {
				if rectsTouch(damage.rects[first], damage.rects[second]) {
					damage.rects[first] = damage.rects[first].Union(damage.rects[second])
					damage.rects = append(damage.rects[:second], damage.rects[second+1:]...)
					merged = true
					break
				}
			}
		}
		if !merged {
			break
		}
	}
	sort.Slice(damage.rects, func(first, second int) bool {
		if damage.rects[first].Min.Y != damage.rects[second].Min.Y {
			return damage.rects[first].Min.Y < damage.rects[second].Min.Y
		}
		return damage.rects[first].Min.X < damage.rects[second].Min.X
	})
}

func rectsTouch(first, second image.Rectangle) bool {
	if !first.Intersect(second).Empty() {
		return true
	}
	expanded := first.Inset(-1)
	return !expanded.Intersect(second).Empty()
}

// Layer is one composited surface at an offset.
type Layer struct {
	Surface *Surface
	X       int
	Y       int
	Opacity float64
}

// Compositor layers surfaces into a destination with damage tracking.
type Compositor struct {
	Width   int
	Height  int
	Surface *Surface

	damage Damage
	layers []Layer
}

// NewCompositor returns a compositor with a transparent destination surface.
func NewCompositor(width, height int) *Compositor {
	return &Compositor{Width: width, Height: height, Surface: NewSurface(width, height)}
}

// BeginFrame starts a new frame and clears the previous damage and layers.
func (compositor *Compositor) BeginFrame() {
	compositor.layers = nil
	compositor.damage.Clear()
}

// AddLayer adds a layer and marks its bounds as damaged.
func (compositor *Compositor) AddLayer(offsetX, offsetY int, source *Surface, opacity float64) {
	if source == nil {
		return
	}
	compositor.layers = append(compositor.layers, Layer{Surface: source, X: offsetX, Y: offsetY, Opacity: opacity})
	compositor.damage.Add(image.Rect(offsetX, offsetY, offsetX+source.Width, offsetY+source.Height))
}

// Invalidate marks a region as damaged without adding a layer.
func (compositor *Compositor) Invalidate(rect image.Rectangle) {
	compositor.damage.Add(rect)
}

// Composite clears damaged regions and draws all layers. It returns the
// bounding damage rectangle.
func (compositor *Compositor) Composite() image.Rectangle {
	damageBounds := compositor.damage.Bounds()
	if damageBounds.Empty() {
		return image.Rectangle{}
	}
	for _, rect := range compositor.damage.Rects() {
		compositor.Surface.clearRect(Rect{
			X:      float64(rect.Min.X),
			Y:      float64(rect.Min.Y),
			Width:  float64(rect.Dx()),
			Height: float64(rect.Dy()),
		})
	}
	for _, layer := range compositor.layers {
		opacity := layer.Opacity
		if opacity <= 0 {
			continue
		}
		if opacity >= 1 {
			compositor.Surface.Blit(layer.X, layer.Y, layer.Surface, SourceOver)
			continue
		}
		compositor.Surface.BlitOpacity(layer.X, layer.Y, layer.Surface, opacity)
	}
	return damageBounds
}

// Damage returns the current damage rectangles.
func (compositor *Compositor) Damage() []image.Rectangle {
	return compositor.damage.Rects()
}

// TileGrid caches a large surface as a grid of independently composited tiles.
type TileGrid struct {
	TileSize int
	Width    int
	Height   int

	tiles map[image.Point]*Surface
	dirty map[image.Point]bool
}

// NewTileGrid creates an empty tile grid.
func NewTileGrid(width, height, tileSize int) *TileGrid {
	if tileSize <= 0 {
		tileSize = 64
	}
	return &TileGrid{
		TileSize: tileSize,
		Width:    width,
		Height:   height,
		tiles:    make(map[image.Point]*Surface),
		dirty:    make(map[image.Point]bool),
	}
}

// TileOrigin returns the destination offset of the tile containing (xPos,yPos).
func (grid *TileGrid) TileOrigin(xPos, yPos int) image.Point {
	column := xPos / grid.TileSize
	row := yPos / grid.TileSize
	return image.Pt(column*grid.TileSize, row*grid.TileSize)
}

// TileAt returns the tile surface containing (xPos,yPos), creating it lazily.
func (grid *TileGrid) TileAt(xPos, yPos int) *Surface {
	origin := grid.TileOrigin(xPos, yPos)
	if tile, ok := grid.tiles[origin]; ok {
		return tile
	}
	width := grid.TileSize
	if remaining := grid.Width - origin.X; remaining < width {
		width = remaining
	}
	height := grid.TileSize
	if remaining := grid.Height - origin.Y; remaining < height {
		height = remaining
	}
	tile := NewSurface(width, height)
	grid.tiles[origin] = tile
	grid.dirty[origin] = true
	return tile
}

// Invalidate marks all tiles intersecting rect as dirty.
func (grid *TileGrid) Invalidate(rect image.Rectangle) {
	firstColumn := rect.Min.X / grid.TileSize
	lastColumn := (rect.Max.X - 1) / grid.TileSize
	firstRow := rect.Min.Y / grid.TileSize
	lastRow := (rect.Max.Y - 1) / grid.TileSize
	for row := firstRow; row <= lastRow; row++ {
		for column := firstColumn; column <= lastColumn; column++ {
			origin := image.Pt(column*grid.TileSize, row*grid.TileSize)
			if grid.tiles[origin] != nil {
				grid.dirty[origin] = true
			}
		}
	}
}

// InvalidateAll marks every existing tile dirty.
func (grid *TileGrid) InvalidateAll() {
	for origin := range grid.tiles {
		grid.dirty[origin] = true
	}
}

// DirtyCount returns the number of dirty tiles.
func (grid *TileGrid) DirtyCount() int {
	return len(grid.dirty)
}

// Composite copies dirty tiles to destination and returns how many were drawn.
func (grid *TileGrid) Composite(destination *Surface) int {
	origins := make([]image.Point, 0, len(grid.dirty))
	for origin := range grid.dirty {
		origins = append(origins, origin)
	}
	sort.Slice(origins, func(first, second int) bool {
		if origins[first].Y != origins[second].Y {
			return origins[first].Y < origins[second].Y
		}
		return origins[first].X < origins[second].X
	})
	for _, origin := range origins {
		tile := grid.tiles[origin]
		if tile == nil {
			continue
		}
		destination.Blit(origin.X, origin.Y, tile, SourceOver)
		delete(grid.dirty, origin)
	}
	return len(origins)
}

func (surface *Surface) clearRect(rect Rect) {
	rect = rect.Intersect(surface.clip)
	if rect.IsEmpty() {
		return
	}
	minX, minY, maxX, maxY := surface.pixelBounds(rect)
	for yPos := minY; yPos < maxY; yPos++ {
		offset := yPos*surface.Stride + minX*4
		for xPos := minX; xPos < maxX; xPos++ {
			surface.Pix[offset] = 0
			surface.Pix[offset+1] = 0
			surface.Pix[offset+2] = 0
			surface.Pix[offset+3] = 0
			offset += 4
		}
	}
}
