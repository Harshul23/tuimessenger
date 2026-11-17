package tui

import (
	"testing"
	"unicode"

	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
	"go.mau.fi/mauview"

	"github.com/whyyor/tuimessenger/internal/tui/widget"
)

func TestTerminalSafe(t *testing.T) {
	if m, _, blank := terminalSafe(0x1f3fb, nil); m != ' ' || !blank {
		t.Errorf("skin tone: got %q blankNext=%v", m, blank)
	}
	if m, c, _ := terminalSafe('⚜', []rune{emojiVS}); m != '⚜' || len(c) != 0 {
		t.Errorf("⚜️: got %q %q, want the bare symbol", m, c)
	}
	if m, _, _ := terminalSafe(zwj, nil); m != ' ' {
		t.Errorf("lone ZWJ: got %q", m)
	}
	// Ordinary combining marks (Devanagari vowel signs) must survive.
	if _, c, _ := terminalSafe('र', []rune{'\u093f'}); len(c) != 1 {
		t.Errorf("Devanagari mark dropped: %q", c)
	}
}

// Ghostty advances zero columns for a non-spacing mark, so a cell starting
// with one would pull the rest of the row left. Draws the real chat name and
// checks none survive as a cell of their own and the text after keeps tcell's column.
func TestDevanagariMarksDontShiftTheRow(t *testing.T) {
	name := "प्रतीक सागर"
	fb := newFakeScreen(30, 1)
	widget.WriteLine(fb, mauview.AlignLeft, name+"X", 0, 0, 30, tcell.StyleDefault)

	gotX := -1
	for x := 0; x < 30; x++ {
		c := fb.cells[x]
		if unicode.Is(unicode.Mn, c.main) {
			t.Errorf("cell %d starts with non-spacing mark %U", x, c.main)
		}
		if c.main == 'X' {
			gotX = x
		}
	}
	if want := 11; gotX != want {
		t.Errorf("X at column %d, want %d", gotX, want)
	}
	if first := fb.cells[0]; first.main != 'प' || len(first.comb) != 1 || first.comb[0] != '\u094d' {
		t.Errorf("virama should stack on प: got %q + %q", first.main, first.comb)
	}
}

// Draws the real room name that misaligned rows and checks nothing the
// terminal would join reaches tcell, and the text after it keeps its column.
func TestJoinedEmojiDontShiftTheRow(t *testing.T) {
	name := "Family👫🧍🏻\u200d♂️"
	fb := newFakeScreen(40, 1)
	widget.WriteLine(fb, mauview.AlignLeft, name+"X", 0, 0, 40, tcell.StyleDefault)
	// tcell places runes one at a time; with nothing left for the terminal to
	// join, the terminal advances the same way, so this is the on-screen column.
	wantX := 0
	for _, r := range name {
		wantX += runewidth.RuneWidth(r)
	}

	gotX := -1
	for x := 0; x < 40; x++ {
		c := fb.cells[x]
		for _, r := range append([]rune{c.main}, c.comb...) {
			if r == zwj || r == emojiVS || isSkinTone(r) {
				t.Errorf("cell %d still carries %U", x, r)
			}
		}
		if c.main == 'X' {
			gotX = x
		}
	}
	if gotX != wantX {
		t.Errorf("X at column %d, want %d", gotX, wantX)
	}
}

// emojiRow draws a reaction row the way DrawReactions does.
type emojiRow struct {
	clearingComponent
	text string
}

func (r *emojiRow) Draw(screen mauview.Screen) {
	w, _ := screen.Size()
	widget.WriteEmojiLine(screen, r.text, 0, 0, w, tcell.StyleDefault.Foreground(tcell.ColorGray))
}

func withWideEmoji(t *testing.T, on bool) {
	orig := widget.WideEmoji
	widget.WideEmoji = on
	t.Cleanup(func() { widget.WideEmoji = orig })
}

// In a terminal that draws ❤️ two columns wide, a reaction ❤️ is the real emoji: two columns in the layout, the
// emoji with its selector, then a cell tcell counts and the terminal doesn't. It survives the selection highlight,
// which reads every cell back and writes it again.
func TestReactionHeartIsTheEmoji(t *testing.T) {
	withWideEmoji(t, true)
	fb := newFakeScreen(12, 1)
	if w := widget.WriteEmojiLine(fb, "👍 2  ❤\ufe0f", 0, 0, 12, tcell.StyleDefault); w != 8 {
		t.Errorf("row width %d, want 8 (❤️ takes two columns)", w)
	}
	check := func(when string) {
		t.Helper()
		heart, tail := fb.cells[6], fb.cells[7]
		if heart.main != '❤' || len(heart.comb) != 1 || heart.comb[0] != emojiVS {
			t.Errorf("%s: heart cell %q %q, want ❤ with the emoji selector", when, heart.main, heart.comb)
		}
		if !tail.tail {
			t.Errorf("%s: next cell %q is not the emoji's right half", when, tail.main)
		}
	}
	check("drawn")
	sel := tcell.StyleDefault.Background(tcell.ColorBlue)
	for x := 0; x < 12; x++ {
		m, c, s, _ := fb.GetContent(x, 0)
		fb.SetContent(x, 0, m, c, s.Background(tcell.ColorBlue))
	}
	check("selected")
	if _, bg, _ := fb.cells[6].style.Decompose(); bg != tcell.ColorBlue || sel == tcell.StyleDefault {
		t.Error("the selection colour should still apply")
	}
}

// Elsewhere (or inside tmux) the heart stays the one-column red text heart, so nothing shifts.
func TestReactionHeartFallsBack(t *testing.T) {
	withWideEmoji(t, false)
	fb := newFakeScreen(12, 1)
	if w := widget.WriteEmojiLine(fb, "❤\ufe0f 2", 0, 0, 12, tcell.StyleDefault); w != 3 {
		t.Errorf("row width %d, want 3", w)
	}
	if c := fb.cells[0]; c.main != '❤' || len(c.comb) != 0 || fb.cells[1].tail {
		t.Errorf("want the bare text heart and no tail, got %q %q", c.main, c.comb)
	}
}

// What reaches tcell: ❤ + selector in one cell; the next cell is locked, so tcell never draws over the emoji's
// right half (the "b" left there proves it wasn't drawn), and unlocked again once something else is there.
func TestReactionHeartReachesTcell(t *testing.T) {
	withWideEmoji(t, true)
	sim := tcell.NewSimulationScreen("UTF-8")
	if err := sim.Init(); err != nil {
		t.Fatal(err)
	}
	defer sim.Fini()
	sim.SetSize(6, 1)
	row := &emojiRow{text: "ab"}
	root := newBufferedRoot(row)
	frame := func(text string) []tcell.SimCell {
		row.text = text
		root.Draw(sim)
		sim.Show()
		cells, _, _ := sim.GetContents()
		return cells
	}
	frame("ab")
	cells := frame("❤️ 2")
	if got := string(cells[0].Runes); got != "❤️" {
		t.Errorf("cell 0 = %q, want the heart emoji", got)
	}
	if got := string(cells[1].Runes); got != "b" {
		t.Errorf("cell 1 = %q: tcell drew over the emoji's right half", got)
	}
	if got := string(cells[3].Runes); got != "2" {
		t.Errorf("cell 3 = %q, want 2 after the two-column heart and a space", got)
	}
	if cells = frame("xy"); string(cells[1].Runes) != "y" {
		t.Errorf("cell 1 = %q after the heart left: the cell should be unlocked and drawn", string(cells[1].Runes))
	}
}

// ❤ is drawn in the emoji's red (it's the ❤️ emoji without its 2-column selector), also in grey reaction rows; on
// a coloured background it keeps its colour.
func TestHeartIsRed(t *testing.T) {
	grey := tcell.StyleDefault.Foreground(tcell.ColorGray)
	fb := newFakeScreen(12, 1)
	mauview.PrintWithStyle(fb, "👍 2  ❤\ufe0f", 0, 0, 12, mauview.AlignLeft, grey)
	var heart *frameCell
	for i := range fb.cells {
		if fb.cells[i].main == '❤' {
			heart = &fb.cells[i]
		}
	}
	if heart == nil {
		t.Fatal("no heart drawn")
	}
	if fg, _, _ := heart.style.Decompose(); fg != emojiHeartRed {
		t.Errorf("reaction ❤ should be red, got %v", fg)
	}
	sel := tcell.StyleDefault.Background(tcell.ColorRed).Foreground(tcell.ColorWhite)
	fb.SetContent(0, 0, '❤', nil, sel)
	if fg, _, _ := fb.cells[0].style.Decompose(); fg != tcell.ColorWhite {
		t.Errorf("on a selection bar the heart keeps its colour, got %v", fg)
	}
}
