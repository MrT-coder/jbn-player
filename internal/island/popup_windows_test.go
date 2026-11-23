//go:build windows

package island

import "testing"

// A 1920x1080 screen whose bottom taskbar (40 px) is excluded from the work area.
var wa = rect{0, 0, 1920, 1040}

func TestPlacePopupAlwaysOpensInTheBottomRightCornerAboveTheTaskbar(t *testing.T) {
	x, y := placePopup(wa, popupW, popupH)
	if x != 1920-popupW-popupMargin || y != 1040-popupH-popupMargin {
		t.Fatalf("got %d,%d", x, y)
	}
}

func TestPlacePopupFollowsTheWorkAreaOfAnotherMonitor(t *testing.T) {
	second := rect{1920, 0, 3840, 1080} // a second monitor to the right, no taskbar
	x, y := placePopup(second, popupW, popupH)
	if x != 3840-popupW-popupMargin || y != 1080-popupH-popupMargin {
		t.Fatalf("got %d,%d", x, y)
	}
}

func TestPlacePopupInATinyWorkAreaStaysAtTheTopLeftMargin(t *testing.T) {
	x, y := placePopup(rect{0, 0, 300, 300}, popupW, popupH)
	if x != popupMargin || y != popupMargin {
		t.Fatalf("tiny work area: got %d,%d", x, y)
	}
}
