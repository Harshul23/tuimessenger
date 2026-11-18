package tui

import (
	"fmt"
	"testing"

	"github.com/gdamore/tcell/v2"
	"go.mau.fi/mauview"
)

// From #46's reproduction: the composer's input is one row (10) with a blank row below it (11). mauview's IsInArea
// counted row 11 as the input, and a double-click there indexed lines[-1] (empty) or lines[1] (one line) and panicked.
func TestDoubleClickBelowComposerDoesntCrash(t *testing.T) {
	for _, text := range []string{"", "hello"} {
		for _, row := range []int{10, 11} {
			if err := doubleClickComposer(text, row); err != nil {
				t.Errorf("text %q, double-click on row %d: %v", text, row, err)
			}
		}
	}
}

func doubleClickComposer(text string, row int) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	sim := tcell.NewSimulationScreen("")
	if err = sim.Init(); err != nil {
		return err
	}
	defer sim.Fini()
	sim.SetSize(40, 20)
	view := &RoomView{
		input:         mauview.NewInputArea(),
		contentScreen: &mauview.ProxyScreen{Parent: sim, OffsetY: 0, Width: 40, Height: 8},
		topicScreen:   &mauview.ProxyScreen{Parent: sim, OffsetY: 19, Width: 40, Height: 1},
		inputScreen:   &mauview.ProxyScreen{Parent: sim, OffsetY: 10, Width: 40, Height: 1},
	}
	view.input.SetText(text)
	for range 2 {
		view.input.Draw(view.inputScreen) // the app draws between events
		view.OnMouseEvent(click{tcell.NewEventMouse(0, row, tcell.Button1, 0)})
	}
	return nil
}

func TestInsideScreenEndsAreExclusive(t *testing.T) {
	ss := &mauview.ProxyScreen{OffsetX: 2, OffsetY: 10, Width: 5, Height: 1}
	for _, c := range []struct {
		x, y int
		want bool
	}{{2, 10, true}, {6, 10, true}, {7, 10, false}, {2, 11, false}, {1, 10, false}, {2, 9, false}} {
		if got := insideScreen(ss, c.x, c.y); got != c.want {
			t.Errorf("insideScreen(%d, %d) = %v, want %v", c.x, c.y, got, c.want)
		}
	}
	if insideScreen(nil, 0, 0) {
		t.Error("no screen: nothing is inside")
	}
}
