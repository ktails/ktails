package models


// Dimensions represents width and height for a pane
type Dimensions struct {
	Width  int
	Height int
}

// NewDimensions creates a new Dimensions with minimum constraints
func NewDimensions(width, height int) Dimensions {
	return Dimensions{
		Width:  max(width, 10),
		Height: max(height, 5),
	}
}

// GetInnerDimensions returns dimensions accounting for frame size
func (d Dimensions) GetInnerDimensions(frameWidth, frameHeight int) Dimensions {
	return Dimensions{
		Width:  max(d.Width-frameWidth, 10),
		Height: max(d.Height-frameHeight-1, 5), // -1 for title bar
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// Pane interface for all panes
type Pane interface {
	SetDimensions(d Dimensions)
	GetDimensions() Dimensions
	SetFocused(bool)
}