package widget

import "testing"

func TestDetectWideEmoji(t *testing.T) {
	cases := []struct {
		env  map[string]string
		want bool
	}{
		{map[string]string{"TERM_PROGRAM": "ghostty"}, true},
		{map[string]string{"TERM": "xterm-ghostty"}, true},
		{map[string]string{"TERM": "xterm-kitty"}, true},
		{map[string]string{"TERM_PROGRAM": "WezTerm"}, true},
		{map[string]string{"TERM_PROGRAM": "ghostty", "TMUX": "/tmp/tmux-501/default,1,0"}, false},
		{map[string]string{"TERM_PROGRAM": "Apple_Terminal"}, false},
		{map[string]string{"TERM_PROGRAM": "iTerm.app"}, false},
	}
	for _, c := range cases {
		if got := detectWideEmoji(func(k string) string { return c.env[k] }); got != c.want {
			t.Errorf("%v: got %v, want %v", c.env, got, c.want)
		}
	}
}

func TestWideEmojiAt(t *testing.T) {
	for s, want := range map[string]bool{"❤\ufe0f": true, "❤": true, "☺\ufe0f": true, "☺": false, "👍": false, "a\ufe0f": false} {
		if got := wideEmojiAt([]rune(s), 0); got != want {
			t.Errorf("%q: got %v, want %v", s, got, want)
		}
	}
}
