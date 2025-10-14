package msgs

import "github.com/charmbracelet/bubbles/table"

type PodTableMsg struct {
	Context string
	Rows    []table.Row
}
