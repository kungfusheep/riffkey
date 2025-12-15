package riffkey

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestParsePattern(t *testing.T) {
	tests := []struct {
		pattern string
		want    []Key
	}{
		// Single keys
		{"j", []Key{{Rune: 'j'}}},
		{"J", []Key{{Rune: 'J'}}},
		{"1", []Key{{Rune: '1'}}},
		{"0", []Key{{Rune: '0'}}},

		// Sequences
		{"gg", []Key{{Rune: 'g'}, {Rune: 'g'}}},
		{"ciw", []Key{{Rune: 'c'}, {Rune: 'i'}, {Rune: 'w'}}},
		{"dd", []Key{{Rune: 'd'}, {Rune: 'd'}}},

		// Vim-style chords
		{"<C-w>", []Key{{Rune: 'w', Mod: ModCtrl}}},
		{"<C-W>", []Key{{Rune: 'W', Mod: ModCtrl}}},
		{"<A-x>", []Key{{Rune: 'x', Mod: ModAlt}}},
		{"<M-x>", []Key{{Rune: 'x', Mod: ModAlt}}}, // M = Meta = Alt
		{"<S-Tab>", []Key{{Special: SpecialTab, Mod: ModShift}}},
		{"<C-A-d>", []Key{{Rune: 'd', Mod: ModCtrl | ModAlt}}},

		// Chord sequences
		{"<C-w><C-j>", []Key{{Rune: 'w', Mod: ModCtrl}, {Rune: 'j', Mod: ModCtrl}}},
		{"<C-w>j", []Key{{Rune: 'w', Mod: ModCtrl}, {Rune: 'j'}}},
		{"g<C-d>", []Key{{Rune: 'g'}, {Rune: 'd', Mod: ModCtrl}}},

		// Special keys
		{"<Esc>", []Key{{Special: SpecialEscape}}},
		{"<CR>", []Key{{Special: SpecialEnter}}},
		{"<Enter>", []Key{{Special: SpecialEnter}}},
		{"<Tab>", []Key{{Special: SpecialTab}}},
		{"<Space>", []Key{{Special: SpecialSpace}}},
		{"<BS>", []Key{{Special: SpecialBackspace}}},

		// Arrow keys
		{"<Up>", []Key{{Special: SpecialUp}}},
		{"<Down>", []Key{{Special: SpecialDown}}},
		{"<Left>", []Key{{Special: SpecialLeft}}},
		{"<Right>", []Key{{Special: SpecialRight}}},

		// Page navigation
		{"<PageUp>", []Key{{Special: SpecialPageUp}}},
		{"<PageDown>", []Key{{Special: SpecialPageDown}}},
		{"<Home>", []Key{{Special: SpecialHome}}},
		{"<End>", []Key{{Special: SpecialEnd}}},

		// Function keys
		{"<F1>", []Key{{Special: SpecialF1}}},
		{"<F12>", []Key{{Special: SpecialF12}}},

		// Modifiers with special keys
		{"<C-Esc>", []Key{{Special: SpecialEscape, Mod: ModCtrl}}},
		{"<C-Up>", []Key{{Special: SpecialUp, Mod: ModCtrl}}},
		{"<A-Left>", []Key{{Special: SpecialLeft, Mod: ModAlt}}},
		{"<C-Space>", []Key{{Special: SpecialSpace, Mod: ModCtrl}}},

		// Complex sequences
		{"g<Esc>", []Key{{Rune: 'g'}, {Special: SpecialEscape}}},
		{"<C-w><Up>", []Key{{Rune: 'w', Mod: ModCtrl}, {Special: SpecialUp}}},
		{"a<Space>b", []Key{{Rune: 'a'}, {Special: SpecialSpace}, {Rune: 'b'}}},
		{"fg<C-w>a<C-l>", []Key{
			{Rune: 'f'},
			{Rune: 'g'},
			{Rune: 'w', Mod: ModCtrl},
			{Rune: 'a'},
			{Rune: 'l', Mod: ModCtrl},
		}},

		// "space" as literal keys (not special)
		{"space", []Key{{Rune: 's'}, {Rune: 'p'}, {Rune: 'a'}, {Rune: 'c'}, {Rune: 'e'}}},

		// Edge cases
		{"", nil},
	}

	for _, tt := range tests {
		t.Run(tt.pattern, func(t *testing.T) {
			got := ParsePattern(tt.pattern)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParsePattern(%q) = %v, want %v", tt.pattern, got, tt.want)
			}
		})
	}
}

func TestKeyString(t *testing.T) {
	tests := []struct {
		key  Key
		want string
	}{
		{Key{Rune: 'j'}, "j"},
		{Key{Rune: 'J'}, "J"},
		{Key{Rune: 'w', Mod: ModCtrl}, "<C-w>"},
		{Key{Rune: 'x', Mod: ModAlt}, "<A-x>"},
		{Key{Rune: 'd', Mod: ModCtrl | ModAlt}, "<C-A-d>"},
		{Key{Special: SpecialEscape}, "<Esc>"},
		{Key{Special: SpecialEnter}, "<CR>"},
		{Key{Special: SpecialEscape, Mod: ModCtrl}, "<C-Esc>"},
		{Key{Special: SpecialTab, Mod: ModShift}, "<S-Tab>"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.key.String()
			if got != tt.want {
				t.Errorf("Key.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRouterSingleKey(t *testing.T) {
	tests := []struct {
		name     string
		patterns []string
		input    []Key
		wantHit  string
	}{
		{
			name:     "simple j",
			patterns: []string{"j"},
			input:    []Key{{Rune: 'j'}},
			wantHit:  "j",
		},
		{
			name:     "uppercase K",
			patterns: []string{"K"},
			input:    []Key{{Rune: 'K'}},
			wantHit:  "K",
		},
		{
			name:     "no match",
			patterns: []string{"j"},
			input:    []Key{{Rune: 'k'}},
			wantHit:  "",
		},
		{
			name:     "ctrl chord",
			patterns: []string{"<C-w>"},
			input:    []Key{{Rune: 'w', Mod: ModCtrl}},
			wantHit:  "<C-w>",
		},
		{
			name:     "escape",
			patterns: []string{"<Esc>"},
			input:    []Key{{Special: SpecialEscape}},
			wantHit:  "<Esc>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewRouter()
			hits := make(map[string]bool)

			for _, p := range tt.patterns {
				pattern := p
				r.Handle(p, func(m Match) {
					hits[pattern] = true
				})
			}

			input := NewInput(r)
			for _, k := range tt.input {
				input.Dispatch(k)
			}

			if tt.wantHit == "" {
				if len(hits) > 0 {
					t.Errorf("expected no hits, got %v", hits)
				}
			} else {
				if !hits[tt.wantHit] {
					t.Errorf("expected hit on %q, got hits: %v", tt.wantHit, hits)
				}
			}
		})
	}
}

func TestRouterSequences(t *testing.T) {
	tests := []struct {
		name     string
		patterns []string
		input    []Key
		wantHit  string
	}{
		{
			name:     "gg sequence",
			patterns: []string{"gg"},
			input:    []Key{{Rune: 'g'}, {Rune: 'g'}},
			wantHit:  "gg",
		},
		{
			name:     "ciw sequence",
			patterns: []string{"ciw"},
			input:    []Key{{Rune: 'c'}, {Rune: 'i'}, {Rune: 'w'}},
			wantHit:  "ciw",
		},
		{
			name:     "partial sequence no match",
			patterns: []string{"gg"},
			input:    []Key{{Rune: 'g'}, {Rune: 'j'}},
			wantHit:  "",
		},
		{
			name:     "chord sequence <C-w><C-j>",
			patterns: []string{"<C-w><C-j>"},
			input:    []Key{{Rune: 'w', Mod: ModCtrl}, {Rune: 'j', Mod: ModCtrl}},
			wantHit:  "<C-w><C-j>",
		},
		{
			name:     "mixed <C-w>j",
			patterns: []string{"<C-w>j"},
			input:    []Key{{Rune: 'w', Mod: ModCtrl}, {Rune: 'j'}},
			wantHit:  "<C-w>j",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewRouter()
			hits := make(map[string]bool)

			for _, p := range tt.patterns {
				pattern := p
				r.Handle(p, func(m Match) {
					hits[pattern] = true
				})
			}

			input := NewInput(r)
			for _, k := range tt.input {
				input.Dispatch(k)
			}

			if tt.wantHit == "" {
				if len(hits) > 0 {
					t.Errorf("expected no hits, got %v", hits)
				}
			} else {
				if !hits[tt.wantHit] {
					t.Errorf("expected hit on %q, got hits: %v", tt.wantHit, hits)
				}
			}
		})
	}
}

func TestCountPrefix(t *testing.T) {
	tests := []struct {
		name      string
		pattern   string
		input     []Key
		wantCount int
		wantHit   bool
	}{
		{
			name:      "no count prefix",
			pattern:   "j",
			input:     []Key{{Rune: 'j'}},
			wantCount: 1,
			wantHit:   true,
		},
		{
			name:      "single digit count",
			pattern:   "j",
			input:     []Key{{Rune: '5'}, {Rune: 'j'}},
			wantCount: 5,
			wantHit:   true,
		},
		{
			name:      "double digit count",
			pattern:   "j",
			input:     []Key{{Rune: '1'}, {Rune: '0'}, {Rune: 'j'}},
			wantCount: 10,
			wantHit:   true,
		},
		{
			name:      "large count",
			pattern:   "gg",
			input:     []Key{{Rune: '9'}, {Rune: '9'}, {Rune: '9'}, {Rune: 'g'}, {Rune: 'g'}},
			wantCount: 999,
			wantHit:   true,
		},
		{
			name:      "zero is not a count (it's a command)",
			pattern:   "0",
			input:     []Key{{Rune: '0'}},
			wantCount: 1, // 0 is treated as a key, not count
			wantHit:   true,
		},
		{
			name:      "count with sequence",
			pattern:   "gg",
			input:     []Key{{Rune: '5'}, {Rune: '0'}, {Rune: 'g'}, {Rune: 'g'}},
			wantCount: 50,
			wantHit:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewRouter()
			var gotCount int
			var hit bool

			r.Handle(tt.pattern, func(m Match) {
				gotCount = m.Count
				hit = true
			})

			input := NewInput(r)
			for _, k := range tt.input {
				input.Dispatch(k)
			}

			if hit != tt.wantHit {
				t.Errorf("hit = %v, want %v", hit, tt.wantHit)
			}
			if gotCount != tt.wantCount {
				t.Errorf("count = %d, want %d", gotCount, tt.wantCount)
			}
		})
	}
}

func TestMatchKeys(t *testing.T) {
	r := NewRouter()

	var gotKeys []Key
	r.Handle("gg", func(m Match) {
		gotKeys = m.Keys
	})

	input := NewInput(r)
	input.Dispatch(Key{Rune: 'g'})
	input.Dispatch(Key{Rune: 'g'})

	if len(gotKeys) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(gotKeys))
	}
	if gotKeys[0].Rune != 'g' || gotKeys[1].Rune != 'g' {
		t.Errorf("expected [g, g], got %v", gotKeys)
	}
}

func TestRouterAmbiguousWithTimeout(t *testing.T) {
	r := NewRouter().Timeout(50 * time.Millisecond)

	var gHit, ggHit atomic.Bool
	var gCount, ggCount atomic.Int32

	r.Handle("g", func(m Match) {
		gHit.Store(true)
		gCount.Store(int32(m.Count))
	})
	r.Handle("gg", func(m Match) {
		ggHit.Store(true)
		ggCount.Store(int32(m.Count))
	})

	t.Run("gg fires immediately", func(t *testing.T) {
		gHit.Store(false)
		ggHit.Store(false)

		input := NewInput(r)
		input.Dispatch(Key{Rune: 'g'})
		input.Dispatch(Key{Rune: 'g'})

		time.Sleep(10 * time.Millisecond)

		if !ggHit.Load() {
			t.Error("expected gg to fire")
		}
		if gHit.Load() {
			t.Error("expected g NOT to fire")
		}
	})

	t.Run("g fires after timeout", func(t *testing.T) {
		gHit.Store(false)
		ggHit.Store(false)

		input := NewInput(r)
		input.Dispatch(Key{Rune: 'g'})

		time.Sleep(10 * time.Millisecond)
		if gHit.Load() {
			t.Error("g fired too early")
		}

		time.Sleep(60 * time.Millisecond)

		if !gHit.Load() {
			t.Error("expected g to fire after timeout")
		}
		if ggHit.Load() {
			t.Error("expected gg NOT to fire")
		}
	})

	t.Run("g cancelled by different key", func(t *testing.T) {
		gHit.Store(false)
		ggHit.Store(false)

		input := NewInput(r)
		input.Dispatch(Key{Rune: 'g'})
		input.Dispatch(Key{Rune: 'j'})

		time.Sleep(60 * time.Millisecond)

		if gHit.Load() {
			t.Error("expected g NOT to fire after cancel")
		}
		if ggHit.Load() {
			t.Error("expected gg NOT to fire")
		}
	})

	t.Run("count prefix with ambiguous", func(t *testing.T) {
		gHit.Store(false)
		ggHit.Store(false)
		gCount.Store(0)
		ggCount.Store(0)

		input := NewInput(r)
		input.Dispatch(Key{Rune: '5'})
		input.Dispatch(Key{Rune: 'g'})
		input.Dispatch(Key{Rune: 'g'})

		time.Sleep(10 * time.Millisecond)

		if !ggHit.Load() {
			t.Error("expected gg to fire with count")
		}
		if ggCount.Load() != 5 {
			t.Errorf("expected count 5, got %d", ggCount.Load())
		}
	})
}

func TestInputPushPop(t *testing.T) {
	normal := NewRouter().Name("normal")
	insert := NewRouter().Name("insert")

	var normalHit, insertHit atomic.Bool

	normal.Handle("i", func(m Match) {
		normalHit.Store(true)
	})
	insert.Handle("i", func(m Match) {
		insertHit.Store(true)
	})

	input := NewInput(normal)

	if input.Current().GetName() != "normal" {
		t.Errorf("expected normal router, got %s", input.Current().GetName())
	}

	input.Dispatch(Key{Rune: 'i'})
	if !normalHit.Load() {
		t.Error("expected normal router to handle i")
	}

	normalHit.Store(false)
	input.Push(insert)

	if input.Current().GetName() != "insert" {
		t.Errorf("expected insert router, got %s", input.Current().GetName())
	}

	input.Dispatch(Key{Rune: 'i'})
	if !insertHit.Load() {
		t.Error("expected insert router to handle i")
	}
	if normalHit.Load() {
		t.Error("expected normal router NOT to handle i")
	}

	insertHit.Store(false)
	input.Pop()

	if input.Current().GetName() != "normal" {
		t.Errorf("expected normal router after pop, got %s", input.Current().GetName())
	}

	input.Dispatch(Key{Rune: 'i'})
	if !normalHit.Load() {
		t.Error("expected normal router to handle i after pop")
	}
}

func TestInputPopAtRoot(t *testing.T) {
	root := NewRouter().Name("root")
	input := NewInput(root)

	input.Pop()
	input.Pop()
	input.Pop()

	if input.Depth() != 1 {
		t.Errorf("expected depth 1 after pops at root, got %d", input.Depth())
	}
}

func TestInputClear(t *testing.T) {
	r := NewRouter().Timeout(100 * time.Millisecond)

	var gHit atomic.Bool

	r.Handle("g", func(m Match) {
		gHit.Store(true)
	})
	r.Handle("gg", func(m Match) {})

	input := NewInput(r)
	input.Dispatch(Key{Rune: 'g'})
	input.Clear()

	time.Sleep(150 * time.Millisecond)

	if gHit.Load() {
		t.Error("expected g NOT to fire after clear")
	}
}

func TestInputFlush(t *testing.T) {
	r := NewRouter().Timeout(100 * time.Millisecond)

	var gHit atomic.Bool
	var gCount atomic.Int32

	r.Handle("g", func(m Match) {
		gHit.Store(true)
		gCount.Store(int32(m.Count))
	})
	r.Handle("gg", func(m Match) {})

	input := NewInput(r)
	input.Dispatch(Key{Rune: '3'})
	input.Dispatch(Key{Rune: 'g'})
	input.Flush()

	if !gHit.Load() {
		t.Error("expected g to fire on flush")
	}
	if gCount.Load() != 3 {
		t.Errorf("expected count 3, got %d", gCount.Load())
	}
}

func TestOverlappingSequences(t *testing.T) {
	tests := []struct {
		name     string
		patterns []string
		input    string
		wantHit  string
	}{
		{
			name:     "longer wins when completed",
			patterns: []string{"ab", "abc"},
			input:    "abc",
			wantHit:  "abc",
		},
		{
			name:     "three levels deep",
			patterns: []string{"a", "ab", "abc", "abcd"},
			input:    "abcd",
			wantHit:  "abcd",
		},
		{
			name:     "vim-style delete word",
			patterns: []string{"d", "dw", "dd", "daw", "diw"},
			input:    "diw",
			wantHit:  "diw",
		},
		{
			name:     "vim-style go commands",
			patterns: []string{"g", "gg", "gt", "gT", "gf", "gd"},
			input:    "gd",
			wantHit:  "gd",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewRouter().Timeout(50 * time.Millisecond)
			hits := make(map[string]bool)

			for _, p := range tt.patterns {
				pattern := p
				r.Handle(p, func(m Match) { hits[pattern] = true })
			}

			input := NewInput(r)
			for _, c := range tt.input {
				input.Dispatch(Key{Rune: c})
			}

			time.Sleep(70 * time.Millisecond)

			if tt.wantHit == "" {
				if len(hits) > 0 {
					t.Errorf("expected no hits, got %v", hits)
				}
			} else {
				if !hits[tt.wantHit] {
					t.Errorf("expected %q to fire, got %v", tt.wantHit, hits)
				}
			}
		})
	}
}

func TestRapidSequenceInput(t *testing.T) {
	r := NewRouter().Timeout(50 * time.Millisecond)

	var ggCount, gtCount atomic.Int32

	r.Handle("g", func(m Match) {})
	r.Handle("gg", func(m Match) { ggCount.Add(1) })
	r.Handle("gt", func(m Match) { gtCount.Add(1) })

	input := NewInput(r)

	// Input: gg gt gg
	for _, c := range "gggtgg" {
		input.Dispatch(Key{Rune: c})
	}

	if ggCount.Load() != 2 {
		t.Errorf("gg fired %d times, want 2", ggCount.Load())
	}
	if gtCount.Load() != 1 {
		t.Errorf("gt fired %d times, want 1", gtCount.Load())
	}
}

func TestCaseSensitivity(t *testing.T) {
	r := NewRouter()

	var lowerHit, upperHit atomic.Bool

	r.Handle("g", func(m Match) { lowerHit.Store(true) })
	r.Handle("G", func(m Match) { upperHit.Store(true) })

	input := NewInput(r)

	input.Dispatch(Key{Rune: 'g'})
	if !lowerHit.Load() {
		t.Error("lowercase g should fire")
	}
	if upperHit.Load() {
		t.Error("uppercase G should not fire for lowercase input")
	}

	lowerHit.Store(false)
	input.Dispatch(Key{Rune: 'G'})
	if lowerHit.Load() {
		t.Error("lowercase g should not fire for uppercase input")
	}
	if !upperHit.Load() {
		t.Error("uppercase G should fire")
	}
}

func TestVeryLongSequence(t *testing.T) {
	r := NewRouter()

	var hit atomic.Bool

	pattern := "abcdefghijklmnopqrst"
	r.Handle(pattern, func(m Match) { hit.Store(true) })

	input := NewInput(r)
	for _, c := range pattern {
		input.Dispatch(Key{Rune: c})
	}

	if !hit.Load() {
		t.Errorf("expected %q to fire", pattern)
	}
}

func TestEmptyRouterDispatch(t *testing.T) {
	r := NewRouter()
	input := NewInput(r)
	handled := input.Dispatch(Key{Rune: 'j'})

	if handled {
		t.Error("empty router should not handle any keys")
	}
}

func TestNilInput(t *testing.T) {
	input := NewInput(nil)
	handled := input.Dispatch(Key{Rune: 'j'})
	if handled {
		t.Error("nil router should not handle keys")
	}
}

func TestConcurrentDispatch(t *testing.T) {
	r := NewRouter()

	var count atomic.Int32

	r.Handle("j", func(m Match) { count.Add(1) })

	input := NewInput(r)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			input.Dispatch(Key{Rune: 'j'})
		}()
	}
	wg.Wait()

	if count.Load() != 100 {
		t.Errorf("expected 100 dispatches, got %d", count.Load())
	}
}

func TestComplexMixedSequences(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		input   []Key
		wantHit bool
	}{
		{
			name:    "fg<C-w>a<C-l>",
			pattern: "fg<C-w>a<C-l>",
			input: []Key{
				{Rune: 'f'},
				{Rune: 'g'},
				{Rune: 'w', Mod: ModCtrl},
				{Rune: 'a'},
				{Rune: 'l', Mod: ModCtrl},
			},
			wantHit: true,
		},
		{
			name:    "wrong modifier in middle",
			pattern: "fg<C-w>a<C-l>",
			input: []Key{
				{Rune: 'f'},
				{Rune: 'g'},
				{Rune: 'w', Mod: ModAlt}, // wrong
				{Rune: 'a'},
				{Rune: 'l', Mod: ModCtrl},
			},
			wantHit: false,
		},
		{
			name:    "alternating chords and plains",
			pattern: "<C-a>b<C-c>d<C-e>",
			input: []Key{
				{Rune: 'a', Mod: ModCtrl},
				{Rune: 'b'},
				{Rune: 'c', Mod: ModCtrl},
				{Rune: 'd'},
				{Rune: 'e', Mod: ModCtrl},
			},
			wantHit: true,
		},
		{
			name:    "special keys interspersed",
			pattern: "a<Esc>b<CR>c",
			input: []Key{
				{Rune: 'a'},
				{Special: SpecialEscape},
				{Rune: 'b'},
				{Special: SpecialEnter},
				{Rune: 'c'},
			},
			wantHit: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewRouter()
			var hit atomic.Bool
			r.Handle(tt.pattern, func(m Match) { hit.Store(true) })

			input := NewInput(r)
			for _, k := range tt.input {
				input.Dispatch(k)
			}

			if hit.Load() != tt.wantHit {
				t.Errorf("hit = %v, want %v", hit.Load(), tt.wantHit)
			}
		})
	}
}

func TestPendingState(t *testing.T) {
	r := NewRouter().Timeout(100 * time.Millisecond)
	r.Handle("g", func(m Match) {})
	r.Handle("gg", func(m Match) {})

	input := NewInput(r)

	// Before any input
	count, keys := input.Pending()
	if count != "" || len(keys) != 0 {
		t.Error("expected empty pending state initially")
	}

	// After count prefix
	input.Dispatch(Key{Rune: '5'})
	count, keys = input.Pending()
	if count != "5" {
		t.Errorf("expected count '5', got %q", count)
	}

	// After partial sequence
	input.Dispatch(Key{Rune: 'g'})
	count, keys = input.Pending()
	if count != "5" {
		t.Errorf("expected count '5', got %q", count)
	}
	if len(keys) != 1 || keys[0].Rune != 'g' {
		t.Errorf("expected [g] in buffer, got %v", keys)
	}
}

// Reader tests

func TestReaderBasicChars(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
		want  Key
	}{
		{
			name:  "lowercase letter",
			input: []byte{'j'},
			want:  Key{Rune: 'j'},
		},
		{
			name:  "uppercase letter",
			input: []byte{'J'},
			want:  Key{Rune: 'J'},
		},
		{
			name:  "digit",
			input: []byte{'5'},
			want:  Key{Rune: '5'},
		},
		{
			name:  "space",
			input: []byte{' '},
			want:  Key{Special: SpecialSpace},
		},
		{
			name:  "enter",
			input: []byte{'\r'},
			want:  Key{Special: SpecialEnter},
		},
		{
			name:  "newline",
			input: []byte{'\n'},
			want:  Key{Special: SpecialEnter},
		},
		{
			name:  "tab",
			input: []byte{'\t'},
			want:  Key{Special: SpecialTab},
		},
		{
			name:  "backspace",
			input: []byte{127},
			want:  Key{Special: SpecialBackspace},
		},
		{
			name:  "ctrl+c",
			input: []byte{3},
			want:  Key{Rune: 'c', Mod: ModCtrl},
		},
		{
			name:  "ctrl+a",
			input: []byte{1},
			want:  Key{Rune: 'a', Mod: ModCtrl},
		},
		{
			name:  "ctrl+z",
			input: []byte{26},
			want:  Key{Rune: 'z', Mod: ModCtrl},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewReader(bytes.NewReader(tt.input))
			got, err := r.ReadKey()
			if err != nil {
				t.Fatalf("ReadKey() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("ReadKey() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestReaderArrowKeys(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
		want  Key
	}{
		{
			name:  "up arrow CSI",
			input: []byte{0x1b, '[', 'A'},
			want:  Key{Special: SpecialUp},
		},
		{
			name:  "down arrow CSI",
			input: []byte{0x1b, '[', 'B'},
			want:  Key{Special: SpecialDown},
		},
		{
			name:  "right arrow CSI",
			input: []byte{0x1b, '[', 'C'},
			want:  Key{Special: SpecialRight},
		},
		{
			name:  "left arrow CSI",
			input: []byte{0x1b, '[', 'D'},
			want:  Key{Special: SpecialLeft},
		},
		{
			name:  "home CSI",
			input: []byte{0x1b, '[', 'H'},
			want:  Key{Special: SpecialHome},
		},
		{
			name:  "end CSI",
			input: []byte{0x1b, '[', 'F'},
			want:  Key{Special: SpecialEnd},
		},
		{
			name:  "up arrow SS3",
			input: []byte{0x1b, 'O', 'A'},
			want:  Key{Special: SpecialUp},
		},
		{
			name:  "down arrow SS3",
			input: []byte{0x1b, 'O', 'B'},
			want:  Key{Special: SpecialDown},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewReader(bytes.NewReader(tt.input))
			got, err := r.ReadKey()
			if err != nil {
				t.Fatalf("ReadKey() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("ReadKey() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestReaderFunctionKeys(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
		want  Key
	}{
		{
			name:  "F1 SS3",
			input: []byte{0x1b, 'O', 'P'},
			want:  Key{Special: SpecialF1},
		},
		{
			name:  "F2 SS3",
			input: []byte{0x1b, 'O', 'Q'},
			want:  Key{Special: SpecialF2},
		},
		{
			name:  "F3 SS3",
			input: []byte{0x1b, 'O', 'R'},
			want:  Key{Special: SpecialF3},
		},
		{
			name:  "F4 SS3",
			input: []byte{0x1b, 'O', 'S'},
			want:  Key{Special: SpecialF4},
		},
		{
			name:  "F1 tilde",
			input: []byte{0x1b, '[', '1', '1', '~'},
			want:  Key{Special: SpecialF1},
		},
		{
			name:  "F5 tilde",
			input: []byte{0x1b, '[', '1', '5', '~'},
			want:  Key{Special: SpecialF5},
		},
		{
			name:  "F6 tilde",
			input: []byte{0x1b, '[', '1', '7', '~'},
			want:  Key{Special: SpecialF6},
		},
		{
			name:  "F11 tilde",
			input: []byte{0x1b, '[', '2', '3', '~'},
			want:  Key{Special: SpecialF11},
		},
		{
			name:  "F12 tilde",
			input: []byte{0x1b, '[', '2', '4', '~'},
			want:  Key{Special: SpecialF12},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewReader(bytes.NewReader(tt.input))
			got, err := r.ReadKey()
			if err != nil {
				t.Fatalf("ReadKey() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("ReadKey() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestReaderTildeSequences(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
		want  Key
	}{
		{
			name:  "home tilde",
			input: []byte{0x1b, '[', '1', '~'},
			want:  Key{Special: SpecialHome},
		},
		{
			name:  "insert",
			input: []byte{0x1b, '[', '2', '~'},
			want:  Key{Special: SpecialInsert},
		},
		{
			name:  "delete",
			input: []byte{0x1b, '[', '3', '~'},
			want:  Key{Special: SpecialDelete},
		},
		{
			name:  "end tilde",
			input: []byte{0x1b, '[', '4', '~'},
			want:  Key{Special: SpecialEnd},
		},
		{
			name:  "page up",
			input: []byte{0x1b, '[', '5', '~'},
			want:  Key{Special: SpecialPageUp},
		},
		{
			name:  "page down",
			input: []byte{0x1b, '[', '6', '~'},
			want:  Key{Special: SpecialPageDown},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewReader(bytes.NewReader(tt.input))
			got, err := r.ReadKey()
			if err != nil {
				t.Fatalf("ReadKey() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("ReadKey() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestReaderModifiedKeys(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
		want  Key
	}{
		{
			name:  "ctrl+up",
			input: []byte{0x1b, '[', '1', ';', '5', 'A'},
			want:  Key{Special: SpecialUp, Mod: ModCtrl},
		},
		{
			name:  "shift+down",
			input: []byte{0x1b, '[', '1', ';', '2', 'B'},
			want:  Key{Special: SpecialDown, Mod: ModShift},
		},
		{
			name:  "alt+right",
			input: []byte{0x1b, '[', '1', ';', '3', 'C'},
			want:  Key{Special: SpecialRight, Mod: ModAlt},
		},
		{
			name:  "ctrl+alt+left",
			input: []byte{0x1b, '[', '1', ';', '7', 'D'},
			want:  Key{Special: SpecialLeft, Mod: ModCtrl | ModAlt},
		},
		{
			name:  "ctrl+shift+up",
			input: []byte{0x1b, '[', '1', ';', '6', 'A'},
			want:  Key{Special: SpecialUp, Mod: ModCtrl | ModShift},
		},
		{
			name:  "ctrl+page_up",
			input: []byte{0x1b, '[', '5', ';', '5', '~'},
			want:  Key{Special: SpecialPageUp, Mod: ModCtrl},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewReader(bytes.NewReader(tt.input))
			got, err := r.ReadKey()
			if err != nil {
				t.Fatalf("ReadKey() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("ReadKey() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestReaderMultipleKeys(t *testing.T) {
	input := []byte{'j', 'k', 'l'}
	r := NewReader(bytes.NewReader(input))

	expected := []Key{
		{Rune: 'j'},
		{Rune: 'k'},
		{Rune: 'l'},
	}

	for i, want := range expected {
		got, err := r.ReadKey()
		if err != nil {
			t.Fatalf("ReadKey() %d error = %v", i, err)
		}
		if got != want {
			t.Errorf("ReadKey() %d = %+v, want %+v", i, got, want)
		}
	}
}

func TestReaderEOF(t *testing.T) {
	r := NewReader(bytes.NewReader([]byte{}))
	_, err := r.ReadKey()
	if err != io.EOF {
		t.Errorf("ReadKey() error = %v, want EOF", err)
	}
}

func TestInputRun(t *testing.T) {
	router := NewRouter()
	var calls []string
	router.Handle("j", func(m Match) { calls = append(calls, "j") })
	router.Handle("k", func(m Match) { calls = append(calls, "k") })

	input := NewInput(router)
	reader := NewReader(bytes.NewReader([]byte{'j', 'k', 'j'}))

	var dispatches int
	err := input.Run(reader, func(handled bool) {
		dispatches++
	})

	if err != io.EOF {
		t.Errorf("Run() error = %v, want EOF", err)
	}

	if dispatches != 3 {
		t.Errorf("dispatches = %d, want 3", dispatches)
	}

	expected := []string{"j", "k", "j"}
	if !reflect.DeepEqual(calls, expected) {
		t.Errorf("calls = %v, want %v", calls, expected)
	}
}

func TestReaderMixedInput(t *testing.T) {
	// Mix of regular keys, escape sequences, and control chars
	input := []byte{
		'j',                    // regular
		0x1b, '[', 'A',         // up arrow
		'k',                    // regular
		0x1b, '[', '5', '~',    // page up
		3,                      // ctrl+c
		0x1b, 'O', 'P',         // F1
		'G',                    // regular uppercase
	}

	expected := []Key{
		{Rune: 'j'},
		{Special: SpecialUp},
		{Rune: 'k'},
		{Special: SpecialPageUp},
		{Rune: 'c', Mod: ModCtrl},
		{Special: SpecialF1},
		{Rune: 'G'},
	}

	r := NewReader(bytes.NewReader(input))
	for i, want := range expected {
		got, err := r.ReadKey()
		if err != nil {
			t.Fatalf("ReadKey() %d error = %v", i, err)
		}
		if got != want {
			t.Errorf("ReadKey() %d = %+v, want %+v", i, got, want)
		}
	}
}

func TestReaderRapidEscapeSequences(t *testing.T) {
	// Multiple escape sequences in rapid succession
	input := []byte{
		0x1b, '[', 'A', // up
		0x1b, '[', 'B', // down
		0x1b, '[', 'C', // right
		0x1b, '[', 'D', // left
		0x1b, '[', 'A', // up again
		0x1b, '[', 'A', // up again
		0x1b, '[', 'A', // up again
	}

	expected := []Key{
		{Special: SpecialUp},
		{Special: SpecialDown},
		{Special: SpecialRight},
		{Special: SpecialLeft},
		{Special: SpecialUp},
		{Special: SpecialUp},
		{Special: SpecialUp},
	}

	r := NewReader(bytes.NewReader(input))
	for i, want := range expected {
		got, err := r.ReadKey()
		if err != nil {
			t.Fatalf("ReadKey() %d error = %v", i, err)
		}
		if got != want {
			t.Errorf("ReadKey() %d = %+v, want %+v", i, got, want)
		}
	}
}

func TestReaderAllControlChars(t *testing.T) {
	// Test all Ctrl+letter combinations (except special ones)
	for i := 1; i <= 26; i++ {
		// Skip special control chars:
		// 8 = Ctrl+H = Backspace (historical)
		// 9 = Ctrl+I = Tab
		// 10 = Ctrl+J = Newline
		// 13 = Ctrl+M = Carriage Return
		if i == 8 || i == 9 || i == 10 || i == 13 {
			continue
		}

		input := []byte{byte(i)}
		r := NewReader(bytes.NewReader(input))
		got, err := r.ReadKey()
		if err != nil {
			t.Fatalf("ReadKey() ctrl+%c error = %v", 'a'+i-1, err)
		}

		want := Key{Rune: rune('a' + i - 1), Mod: ModCtrl}
		if got != want {
			t.Errorf("ReadKey() ctrl+%c = %+v, want %+v", 'a'+i-1, got, want)
		}
	}
}

func TestReaderAllFunctionKeys(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
		want  Special
	}{
		// SS3 style (F1-F4)
		{"F1_SS3", []byte{0x1b, 'O', 'P'}, SpecialF1},
		{"F2_SS3", []byte{0x1b, 'O', 'Q'}, SpecialF2},
		{"F3_SS3", []byte{0x1b, 'O', 'R'}, SpecialF3},
		{"F4_SS3", []byte{0x1b, 'O', 'S'}, SpecialF4},
		// Tilde style (F1-F12)
		{"F1_tilde", []byte{0x1b, '[', '1', '1', '~'}, SpecialF1},
		{"F2_tilde", []byte{0x1b, '[', '1', '2', '~'}, SpecialF2},
		{"F3_tilde", []byte{0x1b, '[', '1', '3', '~'}, SpecialF3},
		{"F4_tilde", []byte{0x1b, '[', '1', '4', '~'}, SpecialF4},
		{"F5_tilde", []byte{0x1b, '[', '1', '5', '~'}, SpecialF5},
		{"F6_tilde", []byte{0x1b, '[', '1', '7', '~'}, SpecialF6},
		{"F7_tilde", []byte{0x1b, '[', '1', '8', '~'}, SpecialF7},
		{"F8_tilde", []byte{0x1b, '[', '1', '9', '~'}, SpecialF8},
		{"F9_tilde", []byte{0x1b, '[', '2', '0', '~'}, SpecialF9},
		{"F10_tilde", []byte{0x1b, '[', '2', '1', '~'}, SpecialF10},
		{"F11_tilde", []byte{0x1b, '[', '2', '3', '~'}, SpecialF11},
		{"F12_tilde", []byte{0x1b, '[', '2', '4', '~'}, SpecialF12},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewReader(bytes.NewReader(tt.input))
			got, err := r.ReadKey()
			if err != nil {
				t.Fatalf("ReadKey() error = %v", err)
			}
			if got.Special != tt.want {
				t.Errorf("ReadKey().Special = %v, want %v", got.Special, tt.want)
			}
		})
	}
}

func TestReaderAllModifierCombinations(t *testing.T) {
	// Terminal modifier encoding: 1 + (shift?1:0) + (alt?2:0) + (ctrl?4:0)
	tests := []struct {
		name     string
		modNum   byte // modifier number in sequence
		wantMod  Modifier
	}{
		{"shift", '2', ModShift},
		{"alt", '3', ModAlt},
		{"shift+alt", '4', ModShift | ModAlt},
		{"ctrl", '5', ModCtrl},
		{"ctrl+shift", '6', ModCtrl | ModShift},
		{"ctrl+alt", '7', ModCtrl | ModAlt},
		{"ctrl+alt+shift", '8', ModCtrl | ModAlt | ModShift},
	}

	for _, tt := range tests {
		t.Run(tt.name+"_arrow", func(t *testing.T) {
			input := []byte{0x1b, '[', '1', ';', tt.modNum, 'A'}
			r := NewReader(bytes.NewReader(input))
			got, err := r.ReadKey()
			if err != nil {
				t.Fatalf("ReadKey() error = %v", err)
			}
			if got.Special != SpecialUp || got.Mod != tt.wantMod {
				t.Errorf("got = %+v, want Special=%v Mod=%v", got, SpecialUp, tt.wantMod)
			}
		})

		t.Run(tt.name+"_pageup", func(t *testing.T) {
			input := []byte{0x1b, '[', '5', ';', tt.modNum, '~'}
			r := NewReader(bytes.NewReader(input))
			got, err := r.ReadKey()
			if err != nil {
				t.Fatalf("ReadKey() error = %v", err)
			}
			if got.Special != SpecialPageUp || got.Mod != tt.wantMod {
				t.Errorf("got = %+v, want Special=%v Mod=%v", got, SpecialPageUp, tt.wantMod)
			}
		})
	}
}

func TestReaderAltKeyVariants(t *testing.T) {
	// Alt+letter combinations
	for c := byte('a'); c <= byte('z'); c++ {
		input := []byte{0x1b, c}
		r := NewReader(bytes.NewReader(input))
		got, err := r.ReadKey()
		if err != nil {
			t.Fatalf("ReadKey() alt+%c error = %v", c, err)
		}
		want := Key{Rune: rune(c), Mod: ModAlt}
		if got != want {
			t.Errorf("ReadKey() alt+%c = %+v, want %+v", c, got, want)
		}
	}

	// Alt+digit
	for c := byte('0'); c <= byte('9'); c++ {
		input := []byte{0x1b, c}
		r := NewReader(bytes.NewReader(input))
		got, err := r.ReadKey()
		if err != nil {
			t.Fatalf("ReadKey() alt+%c error = %v", c, err)
		}
		want := Key{Rune: rune(c), Mod: ModAlt}
		if got != want {
			t.Errorf("ReadKey() alt+%c = %+v, want %+v", c, got, want)
		}
	}
}

func TestReaderLongInputStream(t *testing.T) {
	// Generate a long stream of mixed input
	var input []byte
	var expected []Key

	for i := 0; i < 100; i++ {
		switch i % 5 {
		case 0:
			input = append(input, 'j')
			expected = append(expected, Key{Rune: 'j'})
		case 1:
			input = append(input, 0x1b, '[', 'A')
			expected = append(expected, Key{Special: SpecialUp})
		case 2:
			input = append(input, 3) // ctrl+c
			expected = append(expected, Key{Rune: 'c', Mod: ModCtrl})
		case 3:
			input = append(input, 0x1b, '[', '5', '~')
			expected = append(expected, Key{Special: SpecialPageUp})
		case 4:
			input = append(input, 'G')
			expected = append(expected, Key{Rune: 'G'})
		}
	}

	r := NewReader(bytes.NewReader(input))
	for i, want := range expected {
		got, err := r.ReadKey()
		if err != nil {
			t.Fatalf("ReadKey() %d error = %v", i, err)
		}
		if got != want {
			t.Errorf("ReadKey() %d = %+v, want %+v", i, got, want)
		}
	}
}

func TestReaderNavigationKeys(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
		want  Key
	}{
		// Home/End variations
		{"home_CSI_H", []byte{0x1b, '[', 'H'}, Key{Special: SpecialHome}},
		{"end_CSI_F", []byte{0x1b, '[', 'F'}, Key{Special: SpecialEnd}},
		{"home_tilde_1", []byte{0x1b, '[', '1', '~'}, Key{Special: SpecialHome}},
		{"end_tilde_4", []byte{0x1b, '[', '4', '~'}, Key{Special: SpecialEnd}},
		{"home_tilde_7", []byte{0x1b, '[', '7', '~'}, Key{Special: SpecialHome}},
		{"end_tilde_8", []byte{0x1b, '[', '8', '~'}, Key{Special: SpecialEnd}},
		{"home_SS3", []byte{0x1b, 'O', 'H'}, Key{Special: SpecialHome}},
		{"end_SS3", []byte{0x1b, 'O', 'F'}, Key{Special: SpecialEnd}},
		// Insert/Delete
		{"insert", []byte{0x1b, '[', '2', '~'}, Key{Special: SpecialInsert}},
		{"delete", []byte{0x1b, '[', '3', '~'}, Key{Special: SpecialDelete}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewReader(bytes.NewReader(tt.input))
			got, err := r.ReadKey()
			if err != nil {
				t.Fatalf("ReadKey() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("ReadKey() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestReaderInputRunIntegration(t *testing.T) {
	// Full integration: Reader -> Input -> Router with realistic key sequences
	router := NewRouter()
	var results []string

	router.Handle("j", func(m Match) { results = append(results, fmt.Sprintf("j×%d", m.Count)) })
	router.Handle("k", func(m Match) { results = append(results, fmt.Sprintf("k×%d", m.Count)) })
	router.Handle("gg", func(m Match) { results = append(results, "gg") })
	router.Handle("G", func(m Match) { results = append(results, fmt.Sprintf("G×%d", m.Count)) })
	router.Handle("<C-d>", func(m Match) { results = append(results, "ctrl-d") })
	router.Handle("<Up>", func(m Match) { results = append(results, "up") })
	router.Handle("<PageDown>", func(m Match) { results = append(results, "pgdn") })

	// Simulate vim-like navigation
	input := []byte{
		'g', 'g',               // go to top
		'5', 'j',               // down 5
		0x1b, '[', 'A',         // up arrow
		'1', '0', 'j',          // down 10
		4,                      // ctrl+d (half page down)
		0x1b, '[', '6', '~',    // page down
		'G',                    // go to bottom
	}

	inp := NewInput(router)
	reader := NewReader(bytes.NewReader(input))
	err := inp.Run(reader, nil)
	if err != io.EOF {
		t.Fatalf("Run() error = %v, want EOF", err)
	}

	expected := []string{"gg", "j×5", "up", "j×10", "ctrl-d", "pgdn", "G×1"}
	if !reflect.DeepEqual(results, expected) {
		t.Errorf("results = %v, want %v", results, expected)
	}
}

func TestHasEscapeSequences(t *testing.T) {
	tests := []struct {
		name     string
		patterns []string
		want     bool
	}{
		{
			name:     "no patterns",
			patterns: nil,
			want:     false,
		},
		{
			name:     "only simple keys",
			patterns: []string{"j", "k", "gg", "G", "<C-d>", "<Esc>", "<Enter>", "<Space>"},
			want:     false,
		},
		{
			name:     "with arrow key",
			patterns: []string{"j", "k", "<Up>"},
			want:     true,
		},
		{
			name:     "with F-key",
			patterns: []string{"j", "<F1>"},
			want:     true,
		},
		{
			name:     "with PageUp",
			patterns: []string{"<PageUp>"},
			want:     true,
		},
		{
			name:     "with Alt+key",
			patterns: []string{"j", "<A-x>"},
			want:     true,
		},
		{
			name:     "with Home",
			patterns: []string{"<Home>"},
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewRouter()
			for _, p := range tt.patterns {
				r.Handle(p, func(m Match) {})
			}
			if got := r.HasEscapeSequences(); got != tt.want {
				t.Errorf("HasEscapeSequences() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestReaderSpecialChars(t *testing.T) {
	tests := []struct {
		name  string
		input byte
		want  Key
	}{
		{"space", ' ', Key{Special: SpecialSpace}},
		{"tab", '\t', Key{Special: SpecialTab}},
		{"enter_cr", '\r', Key{Special: SpecialEnter}},
		{"enter_lf", '\n', Key{Special: SpecialEnter}},
		{"backspace_127", 127, Key{Special: SpecialBackspace}},
		{"backspace_8", 8, Key{Special: SpecialBackspace}},
		{"ctrl_space", 0, Key{Rune: ' ', Mod: ModCtrl}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewReader(bytes.NewReader([]byte{tt.input}))
			got, err := r.ReadKey()
			if err != nil {
				t.Fatalf("ReadKey() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("ReadKey() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestAliases(t *testing.T) {
	tests := []struct {
		name     string
		aliases  map[string]string
		patterns []string
		input    string
		want     []string // expected actions triggered
	}{
		{
			name:     "simple leader",
			aliases:  map[string]string{"Leader": ","},
			patterns: []string{"<Leader>f", "<Leader>b"},
			input:    ",f,b",
			want:     []string{"<Leader>f", "<Leader>b"},
		},
		{
			name:     "chord alias",
			aliases:  map[string]string{"Nav": "<C-w>"},
			patterns: []string{"<Nav>j", "<Nav>k"},
			input:    string([]byte{23}) + "j" + string([]byte{23}) + "k", // Ctrl+w = 23
			want:     []string{"<Nav>j", "<Nav>k"},
		},
		{
			name:     "multiple aliases",
			aliases:  map[string]string{"Leader": ",", "LocalLeader": "\\"},
			patterns: []string{"<Leader>x", "<LocalLeader>y"},
			input:    ",x\\y",
			want:     []string{"<Leader>x", "<LocalLeader>y"},
		},
		{
			name:     "case insensitive alias",
			aliases:  map[string]string{"Leader": ","},
			patterns: []string{"<LEADER>f", "<leader>b"},
			input:    ",f,b",
			want:     []string{"<LEADER>f", "<leader>b"},
		},
		{
			name:     "alias in middle of pattern",
			aliases:  map[string]string{"Nav": "<C-w>"},
			patterns: []string{"g<Nav>j"},
			input:    "g" + string([]byte{23}) + "j",
			want:     []string{"g<Nav>j"},
		},
		{
			name:     "no recursive expansion",
			aliases:  map[string]string{"A": "<B>", "B": "x"},
			patterns: []string{"<A>"},
			input:    "x",
			want:     []string{}, // <A> expands to <B> (which parses as 'B'), so 'x' won't match
		},
		{
			name:     "chained alias expands once",
			aliases:  map[string]string{"A": "<B>", "B": "x"},
			patterns: []string{"<A>"},
			input:    "B", // <A> expands to <B> which parses as 'B', so 'B' matches
			want:     []string{"<A>"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewRouter()
			for k, v := range tt.aliases {
				r.SetAlias(k, v)
			}

			var triggered []string
			var mu sync.Mutex
			for _, p := range tt.patterns {
				pat := p
				r.Handle(p, func(m Match) {
					mu.Lock()
					triggered = append(triggered, pat)
					mu.Unlock()
				})
			}

			// Feed input
			input := NewInput(r)
			reader := NewReader(bytes.NewReader([]byte(tt.input)))
			for {
				key, err := reader.ReadKey()
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Fatalf("ReadKey error: %v", err)
				}
				input.Dispatch(key)
			}

			if len(triggered) != len(tt.want) {
				t.Errorf("got %v triggered, want %v", triggered, tt.want)
				return
			}
			for i := range triggered {
				if triggered[i] != tt.want[i] {
					t.Errorf("triggered[%d] = %q, want %q", i, triggered[i], tt.want[i])
				}
			}
		})
	}
}

func TestAliasExpandsOnce(t *testing.T) {
	// Ensure aliases only expand once (no recursive expansion)
	r := NewRouter()
	r.SetAlias("A", "<B>")
	r.SetAlias("B", "x")

	var triggered bool
	r.Handle("<A>", func(m Match) {
		triggered = true
	})

	// <A> expands to <B>, but <B> does NOT further expand to x
	// <B> as a key pattern parses as just 'B' since B isn't a special key or modifier
	input := NewInput(r)
	reader := NewReader(bytes.NewReader([]byte("B")))
	key, _ := reader.ReadKey()
	input.Dispatch(key)

	if !triggered {
		t.Error("<A> should have expanded to <B> which parses as 'B'")
	}
}

func TestSetAliasChaining(t *testing.T) {
	// Test that SetAlias returns the router for chaining
	r := NewRouter().
		SetAlias("Leader", ",").
		SetAlias("LocalLeader", "\\")

	r.Handle("<Leader>f", func(m Match) {})
	r.Handle("<LocalLeader>g", func(m Match) {})

	// Just verify it compiles and doesn't panic
	if r.aliases == nil || len(r.aliases) != 2 {
		t.Error("expected 2 aliases")
	}
}

func TestHandleNamed(t *testing.T) {
	r := NewRouter()

	var scrollHit, topHit bool
	r.HandleNamed("scroll_down", "j", func(m Match) { scrollHit = true })
	r.HandleNamed("go_to_top", "gg", func(m Match) { topHit = true })

	input := NewInput(r)
	input.Dispatch(Key{Rune: 'j'})

	if !scrollHit {
		t.Error("scroll_down should have fired")
	}

	input.Dispatch(Key{Rune: 'g'})
	input.Dispatch(Key{Rune: 'g'})

	if !topHit {
		t.Error("go_to_top should have fired")
	}
}

func TestBindings(t *testing.T) {
	r := NewRouter()
	r.HandleNamed("scroll_down", "j", func(m Match) {})
	r.HandleNamed("scroll_up", "k", func(m Match) {})
	r.HandleNamed("go_to_top", "gg", func(m Match) {})

	bindings := r.Bindings()
	if len(bindings) != 3 {
		t.Fatalf("expected 3 bindings, got %d", len(bindings))
	}

	// Check order is preserved
	if bindings[0].Name != "scroll_down" {
		t.Errorf("expected first binding to be scroll_down, got %s", bindings[0].Name)
	}
	if bindings[1].Name != "scroll_up" {
		t.Errorf("expected second binding to be scroll_up, got %s", bindings[1].Name)
	}
	if bindings[2].Name != "go_to_top" {
		t.Errorf("expected third binding to be go_to_top, got %s", bindings[2].Name)
	}

	// Check patterns
	if bindings[0].Pattern != "j" || bindings[0].DefaultPattern != "j" {
		t.Errorf("scroll_down pattern mismatch")
	}
}

func TestRebind(t *testing.T) {
	r := NewRouter()

	var hit bool
	r.HandleNamed("scroll_down", "j", func(m Match) { hit = true })

	// Rebind to different key
	if !r.Rebind("scroll_down", "n") {
		t.Error("Rebind should succeed")
	}

	// Old key should not work
	input := NewInput(r)
	input.Dispatch(Key{Rune: 'j'})
	if hit {
		t.Error("old binding 'j' should not fire after rebind")
	}

	// New key should work
	input.Dispatch(Key{Rune: 'n'})
	if !hit {
		t.Error("new binding 'n' should fire")
	}

	// Check Bindings() reflects the change
	bindings := r.Bindings()
	if bindings[0].Pattern != "n" {
		t.Errorf("expected pattern 'n', got %s", bindings[0].Pattern)
	}
	if bindings[0].DefaultPattern != "j" {
		t.Errorf("expected default pattern 'j', got %s", bindings[0].DefaultPattern)
	}
}

func TestReset(t *testing.T) {
	r := NewRouter()

	var hit bool
	r.HandleNamed("scroll_down", "j", func(m Match) { hit = true })
	r.Rebind("scroll_down", "n")

	// Reset to default
	if !r.Reset("scroll_down") {
		t.Error("Reset should succeed")
	}

	// Original key should work again
	input := NewInput(r)
	input.Dispatch(Key{Rune: 'j'})
	if !hit {
		t.Error("original binding 'j' should fire after reset")
	}
}

func TestResetAll(t *testing.T) {
	r := NewRouter()

	r.HandleNamed("scroll_down", "j", func(m Match) {})
	r.HandleNamed("scroll_up", "k", func(m Match) {})

	r.Rebind("scroll_down", "n")
	r.Rebind("scroll_up", "p")

	r.ResetAll()

	bindings := r.Bindings()
	for _, b := range bindings {
		if b.Pattern != b.DefaultPattern {
			t.Errorf("%s: pattern %s != default %s after ResetAll", b.Name, b.Pattern, b.DefaultPattern)
		}
	}
}

func TestBindingsMap(t *testing.T) {
	r := NewRouter()
	r.HandleNamed("scroll_down", "j", func(m Match) {})
	r.HandleNamed("scroll_up", "k", func(m Match) {})
	r.Rebind("scroll_down", "n")

	m := r.BindingsMap()
	if m["scroll_down"] != "n" {
		t.Errorf("expected scroll_down='n', got %s", m["scroll_down"])
	}
	if m["scroll_up"] != "k" {
		t.Errorf("expected scroll_up='k', got %s", m["scroll_up"])
	}
}

func TestApplyBindings(t *testing.T) {
	r := NewRouter()

	var jHit, nHit bool
	r.HandleNamed("scroll_down", "j", func(m Match) { jHit = true })

	// Apply bindings from map
	r.ApplyBindings(map[string]string{
		"scroll_down": "n",
		"unknown":     "x", // Should be silently ignored
	})

	input := NewInput(r)
	input.Dispatch(Key{Rune: 'j'})
	if jHit {
		t.Error("old binding should not fire")
	}

	input.Dispatch(Key{Rune: 'n'})
	nHit = jHit // jHit gets set by the handler
	if !nHit {
		t.Error("new binding should fire")
	}
}

func TestWriteDefaultBindings(t *testing.T) {
	r := NewRouter()
	r.HandleNamed("scroll_down", "j", func(m Match) {})
	r.HandleNamed("go_to_top", "gg", func(m Match) {})

	var buf bytes.Buffer
	if err := r.WriteDefaultBindings(&buf, "myapp"); err != nil {
		t.Fatalf("WriteDefaultBindings error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "[myapp]") {
		t.Error("expected [myapp] section header")
	}
	if !strings.Contains(output, "# scroll_down = \"j\"") {
		t.Error("expected commented scroll_down binding")
	}
	if !strings.Contains(output, "# go_to_top = \"gg\"") {
		t.Error("expected commented go_to_top binding")
	}
}

func TestLoadBindingsFromString(t *testing.T) {
	r := NewRouter()

	var jHit, kHit bool
	r.HandleNamed("scroll_down", "j", func(m Match) { jHit = true })
	r.HandleNamed("scroll_up", "k", func(m Match) { kHit = true })

	// Create temp config file
	configContent := `
[global]
scroll_down = "n"

[myapp]
scroll_up = "p"
`
	tmpFile, err := os.CreateTemp("", "riffkey-test-*.toml")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(configContent); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	tmpFile.Close()

	// Load config
	if err := r.LoadBindingsFrom(tmpFile.Name(), "myapp"); err != nil {
		t.Fatalf("LoadBindingsFrom error: %v", err)
	}

	// Check bindings were applied
	bindings := r.BindingsMap()
	if bindings["scroll_down"] != "n" {
		t.Errorf("expected scroll_down='n' from global, got %s", bindings["scroll_down"])
	}
	if bindings["scroll_up"] != "p" {
		t.Errorf("expected scroll_up='p' from myapp section, got %s", bindings["scroll_up"])
	}

	// Verify the new bindings work
	input := NewInput(r)
	input.Dispatch(Key{Rune: 'n'})
	if !jHit {
		t.Error("scroll_down rebound to 'n' should fire")
	}
	input.Dispatch(Key{Rune: 'p'})
	if !kHit {
		t.Error("scroll_up rebound to 'p' should fire")
	}
}

func TestLoadBindingsWithAliases(t *testing.T) {
	r := NewRouter()

	var hit bool
	r.HandleNamed("find_files", ",f", func(m Match) { hit = true })

	// Create temp config with aliases
	configContent := `
[aliases]
Leader = ","

[myapp]
find_files = "<Leader>f"
`
	tmpFile, err := os.CreateTemp("", "riffkey-alias-test-*.toml")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(configContent); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	tmpFile.Close()

	// Load config
	if err := r.LoadBindingsFrom(tmpFile.Name(), "myapp"); err != nil {
		t.Fatalf("LoadBindingsFrom error: %v", err)
	}

	// Verify alias was applied and binding works
	input := NewInput(r)
	input.Dispatch(Key{Rune: ','})
	input.Dispatch(Key{Rune: 'f'})
	if !hit {
		t.Error("<Leader>f should expand to ,f and fire")
	}
}

func TestLoadBindingsMissingFile(t *testing.T) {
	r := NewRouter()
	r.HandleNamed("scroll_down", "j", func(m Match) {})

	// Should not error on missing file
	if err := r.LoadBindingsFrom("/nonexistent/path/config.toml", "myapp"); err != nil {
		t.Errorf("LoadBindingsFrom should not error on missing file: %v", err)
	}

	// Binding should still be at default
	if r.BindingsMap()["scroll_down"] != "j" {
		t.Error("binding should remain at default when config missing")
	}
}

func TestConfigPath(t *testing.T) {
	path := ConfigPath()
	if path == "" {
		t.Skip("could not determine config path")
	}

	if !strings.Contains(path, "riffkey.toml") {
		t.Errorf("config path should end with riffkey.toml, got %s", path)
	}
}
