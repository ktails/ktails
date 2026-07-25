package pages

import "testing"

// TestWantsDetailOverlay checks the §8.4 threshold: a bottom split needs at
// least detailMinRows for the Detail pane after listMinRows is reserved for
// the list; below that, the pane should render as a full-screen overlay
// instead of two half-broken panes.
func TestWantsDetailOverlay(t *testing.T) {
	cases := []struct {
		tableH int
		want   bool
	}{
		{14, true},  // available (12) < detailMinRows+listMinRows (13): overlay
		{15, false}, // available (13) meets the threshold exactly: split
		{40, false}, // plenty of room: split
	}
	for _, tc := range cases {
		m := &MainPage{tableH: tc.tableH}
		if got := m.wantsDetailOverlay(); got != tc.want {
			t.Errorf("wantsDetailOverlay() at tableH=%d = %v, want %v", tc.tableH, got, tc.want)
		}
	}
}
