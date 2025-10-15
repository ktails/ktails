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
		Height: max(height, 3), // titled pane requires at least 1 for title + 2 for body/border
	}
}

// GetInnerDimensions returns dimensions accounting for frame size
func (d Dimensions) GetInnerDimensions(frameWidth, frameHeight int) Dimensions {
	return Dimensions{
		Width:  max(d.Width-frameWidth, 10),
		Height: max(d.Height-frameHeight, 1), // inner body can be at least one row
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
