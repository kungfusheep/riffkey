package riffkey

import (
	"bytes"
	"testing"
)

// opt+Backspace on macOS sends ESC then DEL (0x7f) — and some terminals ESC then BS
// (0x08). Both must parse as Alt+Backspace, NOT a lone Escape: a lone Escape closes
// modal dialogs and discards the user's text (the reported bug).
func TestReadAltBackspaceNotEscape(t *testing.T) {
	for _, in := range [][]byte{{27, 127}, {27, 8}} {
		r := NewReader(bytes.NewReader(in))
		got, err := r.ReadKey()
		if err != nil {
			t.Fatalf("ReadKey(%v) error: %v", in, err)
		}
		want := Key{Special: SpecialBackspace, Mod: ModAlt}
		if got != want {
			t.Fatalf("ReadKey(%v) = %+v, want %+v (must not degrade to Escape)", in, got, want)
		}
	}
}

// Alt+Backspace deletes the word before the cursor (like Ctrl+W), so opt+Backspace is
// a useful editing shortcut instead of a dialog-closing footgun.
func TestTextHandlerAltBackspaceDeletesWord(t *testing.T) {
	val := "hello world"
	cur := len(val)
	th := NewTextHandler(&val, &cur)

	if !th.HandleKey(Key{Special: SpecialBackspace, Mod: ModAlt}) {
		t.Fatal("Alt+Backspace should be handled")
	}
	if val != "hello " || cur != len("hello ") {
		t.Fatalf("after Alt+Backspace: val=%q cur=%d, want %q/%d", val, cur, "hello ", len("hello "))
	}

	// a second one removes "hello " (the word + its trailing spaces)
	th.HandleKey(Key{Special: SpecialBackspace, Mod: ModAlt})
	if val != "" || cur != 0 {
		t.Fatalf("after second Alt+Backspace: val=%q cur=%d, want empty", val, cur)
	}

	// plain Backspace still deletes a single char (regression guard)
	val, cur = "ab", 2
	th = NewTextHandler(&val, &cur)
	th.HandleKey(Key{Special: SpecialBackspace})
	if val != "a" || cur != 1 {
		t.Fatalf("plain Backspace: val=%q cur=%d, want \"a\"/1", val, cur)
	}

	// Ctrl+W still deletes a word (shares wordStartBefore)
	val, cur = "one two", len("one two")
	th = NewTextHandler(&val, &cur)
	th.HandleKey(Key{Rune: 'w', Mod: ModCtrl})
	if val != "one " {
		t.Fatalf("Ctrl+W: val=%q, want %q", val, "one ")
	}
}
