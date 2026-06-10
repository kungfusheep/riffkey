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

// opt+Enter on macOS sends ESC then CR — it must parse as Alt+Enter, NOT a lone
// Escape (which closes modal dialogs, same trap as opt+Backspace).
func TestReadAltEnterNotEscape(t *testing.T) {
	for _, in := range [][]byte{{27, 13}, {27, 10}} {
		r := NewReader(bytes.NewReader(in))
		got, err := r.ReadKey()
		if err != nil {
			t.Fatalf("ReadKey(%v) error: %v", in, err)
		}
		want := Key{Special: SpecialEnter, Mod: ModAlt}
		if got != want {
			t.Fatalf("ReadKey(%v) = %+v, want %+v", in, got, want)
		}
	}
}

// Alt+Enter and Ctrl+J insert a newline ONLY when the field opts in via
// AllowNewlines — a single-line field (filter query) must never get a '\n'.
func TestTextHandlerNewlinesGated(t *testing.T) {
	val, cur := "ab", 1
	th := NewTextHandler(&val, &cur)

	// default (single-line): both combos unhandled, value untouched
	if th.HandleKey(Key{Special: SpecialEnter, Mod: ModAlt}) {
		t.Fatal("Alt+Enter should be unhandled without AllowNewlines")
	}
	if th.HandleKey(Key{Rune: 'j', Mod: ModCtrl}) {
		t.Fatal("Ctrl+J should be unhandled without AllowNewlines")
	}
	if val != "ab" {
		t.Fatalf("single-line value corrupted: %q", val)
	}

	// multiline: both insert '\n' at the cursor
	th.AllowNewlines = true
	if !th.HandleKey(Key{Special: SpecialEnter, Mod: ModAlt}) {
		t.Fatal("Alt+Enter should insert a newline with AllowNewlines")
	}
	if val != "a\nb" || cur != 2 {
		t.Fatalf("after Alt+Enter: val=%q cur=%d, want \"a\\nb\"/2", val, cur)
	}
	if !th.HandleKey(Key{Rune: 'j', Mod: ModCtrl}) {
		t.Fatal("Ctrl+J should insert a newline with AllowNewlines")
	}
	if val != "a\n\nb" || cur != 3 {
		t.Fatalf("after Ctrl+J: val=%q cur=%d, want \"a\\n\\nb\"/3", val, cur)
	}
}
