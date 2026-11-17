package widget

import (
	"os"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
	"go.mau.fi/mauview"
)

// WideTail fills the second column of an emoji WriteEmojiLine draws two columns wide; the TUI's frame buffer keeps
// tcell from drawing that column, which the emoji covers on screen.
const WideTail = '\uE000'

const emojiVS = '\uFE0F'

// WideEmoji says the terminal draws an emoji with the variation selector (❤️) two columns wide, as Unicode asks,
// while tcell counts it as one. Known for Ghostty, kitty and WezTerm; not inside tmux, which measures by itself.
var WideEmoji = detectWideEmoji(os.Getenv)

func detectWideEmoji(getenv func(string) string) bool {
	if getenv("TMUX") != "" {
		return false
	}
	term, prog := getenv("TERM"), getenv("TERM_PROGRAM")
	return prog == "ghostty" || prog == "WezTerm" || getenv("KITTY_WINDOW_ID") != "" ||
		strings.Contains(term, "ghostty") || strings.Contains(term, "kitty")
}

// wideEmojiAt reports an emoji that is one column to tcell but two on screen: a narrow symbol followed by the
// emoji variation selector (❤️ ☺️ ✌️), or a bare ❤ (reaction keys sometimes lack the selector).
func wideEmojiAt(runes []rune, i int) bool {
	r := runes[i]
	if r < 0x2000 || runewidth.RuneWidth(r) != 1 {
		return false
	}
	return r == '\u2764' || (i+1 < len(runes) && runes[i+1] == emojiVS)
}

// WriteEmojiLine is WriteLine (left-aligned) for short emoji text such as reactions: with WideEmoji, an emoji like
// ❤️ takes the two columns the terminal gives it, instead of becoming the one-column text symbol. Returns the width.
func WriteEmojiLine(screen mauview.Screen, line string, x, y, maxWidth int, style tcell.Style) int {
	runes := []rune(line)
	offset := 0
	for i := 0; i < len(runes) && offset < maxWidth; i++ {
		ch := runes[i]
		if WideEmoji && wideEmojiAt(runes, i) && offset+2 <= maxWidth {
			screen.SetContent(x+offset, y, ch, nil, style)
			screen.SetContent(x+offset+1, y, WideTail, nil, style)
			offset += 2
			continue
		}
		chWidth := runewidth.RuneWidth(ch)
		if chWidth == 0 {
			continue
		}
		if offset+chWidth > maxWidth {
			break
		}
		for localOffset := 0; localOffset < chWidth; localOffset++ {
			screen.SetContent(x+offset+localOffset, y, ch, nil, style)
		}
		offset += chWidth
	}
	return offset
}
