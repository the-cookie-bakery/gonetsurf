package geometry

// Mask is an anti-aliased coverage buffer. Each value is in [0, 1].
type Mask struct {
	Width    int
	Height   int
	Coverage []float64
}

// NewMask returns an empty coverage mask.
func NewMask(width, height int) *Mask {
	if width < 0 {
		width = 0
	}
	if height < 0 {
		height = 0
	}
	return &Mask{Width: width, Height: height, Coverage: make([]float64, width*height)}
}

// At returns the coverage at (xPos, yPos).
func (mask *Mask) At(xPos, yPos int) float64 {
	if mask == nil || xPos < 0 || yPos < 0 || xPos >= mask.Width || yPos >= mask.Height {
		return 0
	}
	return mask.Coverage[yPos*mask.Width+xPos]
}

// Union combines other into mask using coverage composition.
func (mask *Mask) Union(other *Mask) {
	if mask == nil || other == nil || mask.Width != other.Width || mask.Height != other.Height {
		return
	}
	for index := range mask.Coverage {
		first := mask.Coverage[index]
		second := other.Coverage[index]
		mask.Coverage[index] = 1 - (1-first)*(1-second)
	}
}
