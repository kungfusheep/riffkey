package riffkey

import (
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/BurntSushi/toml"
)

// Modifier represents key modifiers (Ctrl, Alt, Shift).
type Modifier uint8

const (
	ModNone Modifier = 0
	ModCtrl Modifier = 1 << iota
	ModAlt
	ModShift
)

// Special represents special (non-printable) keys.
type Special uint8

const (
	SpecialNone Special = iota
	SpecialEscape
	SpecialEnter
	SpecialTab
	SpecialSpace
	SpecialBackspace
	SpecialUp
	SpecialDown
	SpecialLeft
	SpecialRight
	SpecialHome
	SpecialEnd
	SpecialPageUp
	SpecialPageDown
	SpecialInsert
	SpecialDelete
	SpecialF1
	SpecialF2
	SpecialF3
	SpecialF4
	SpecialF5
	SpecialF6
	SpecialF7
	SpecialF8
	SpecialF9
	SpecialF10
	SpecialF11
	SpecialF12
)

// Key represents a single keypress with optional modifiers.
type Key struct {
	Rune    rune
	Mod     Modifier
	Special Special
}

// String returns a vim-style representation of the key.
func (k Key) String() string {
	if k.Special == SpecialNone && k.Mod == ModNone && k.Rune != 0 {
		return string(k.Rune)
	}

	var parts []string
	if k.Mod&ModCtrl != 0 {
		parts = append(parts, "C")
	}
	if k.Mod&ModAlt != 0 {
		parts = append(parts, "A")
	}
	if k.Mod&ModShift != 0 {
		parts = append(parts, "S")
	}

	var keyPart string
	if k.Special != SpecialNone {
		keyPart = specialToVim[k.Special]
	} else if k.Rune != 0 {
		keyPart = string(k.Rune)
	}

	if len(parts) > 0 || k.Special != SpecialNone {
		return "<" + strings.Join(append(parts, keyPart), "-") + ">"
	}
	return keyPart
}

var specialToVim = map[Special]string{
	SpecialEscape:    "Esc",
	SpecialEnter:     "CR",
	SpecialTab:       "Tab",
	SpecialSpace:     "Space",
	SpecialBackspace: "BS",
	SpecialUp:        "Up",
	SpecialDown:      "Down",
	SpecialLeft:      "Left",
	SpecialRight:     "Right",
	SpecialHome:      "Home",
	SpecialEnd:       "End",
	SpecialPageUp:    "PageUp",
	SpecialPageDown:  "PageDown",
	SpecialInsert:    "Insert",
	SpecialDelete:    "Del",
	SpecialF1:        "F1",
	SpecialF2:        "F2",
	SpecialF3:        "F3",
	SpecialF4:        "F4",
	SpecialF5:        "F5",
	SpecialF6:        "F6",
	SpecialF7:        "F7",
	SpecialF8:        "F8",
	SpecialF9:        "F9",
	SpecialF10:       "F10",
	SpecialF11:       "F11",
	SpecialF12:       "F12",
}

var vimToSpecial = map[string]Special{
	"esc":       SpecialEscape,
	"escape":    SpecialEscape,
	"cr":        SpecialEnter,
	"enter":     SpecialEnter,
	"return":    SpecialEnter,
	"tab":       SpecialTab,
	"space":     SpecialSpace,
	"bs":        SpecialBackspace,
	"backspace": SpecialBackspace,
	"up":        SpecialUp,
	"down":      SpecialDown,
	"left":      SpecialLeft,
	"right":     SpecialRight,
	"home":      SpecialHome,
	"end":       SpecialEnd,
	"pageup":    SpecialPageUp,
	"pagedown":  SpecialPageDown,
	"insert":    SpecialInsert,
	"del":       SpecialDelete,
	"delete":    SpecialDelete,
	"f1":        SpecialF1,
	"f2":        SpecialF2,
	"f3":        SpecialF3,
	"f4":        SpecialF4,
	"f5":        SpecialF5,
	"f6":        SpecialF6,
	"f7":        SpecialF7,
	"f8":        SpecialF8,
	"f9":        SpecialF9,
	"f10":       SpecialF10,
	"f11":       SpecialF11,
	"f12":       SpecialF12,
}

// Match contains information about a matched key sequence.
type Match struct {
	Keys  []Key // The matched key sequence (without count prefix digits)
	Count int   // Count prefix (defaults to 1 if not specified)
}

// Handler is a function that handles a matched key sequence.
type Handler func(m Match)

// Binding represents a named key binding with its current and default patterns.
type Binding struct {
	Name           string // Semantic action name (e.g., "scroll_down")
	Pattern        string // Current pattern (after rebinding)
	DefaultPattern string // Original default pattern
}

// namedBinding stores internal binding info.
type namedBinding struct {
	defaultPattern string
	currentPattern string
	handler        Handler
}

// Router matches key patterns to handlers.
type Router struct {
	root               *trieNode
	timeout            time.Duration
	name               string
	hasEscapeSequences bool              // true if any registered pattern uses keys that generate escape sequences
	aliases            map[string]string // user-defined pattern aliases (e.g., "Leader" -> ",")
	namedBindings      map[string]*namedBinding
	bindingOrder       []string // preserve registration order for Bindings()
}

type trieNode struct {
	children map[Key]*trieNode
	handler  Handler
}

// NewRouter creates a new Router with default settings.
func NewRouter() *Router {
	return &Router{
		root:          &trieNode{children: make(map[Key]*trieNode)},
		timeout:       2 * time.Second,
		namedBindings: make(map[string]*namedBinding),
	}
}

// HasEscapeSequences returns true if any registered pattern uses keys
// that generate terminal escape sequences (arrows, F-keys, etc.).
// This can be used to optimize input reading by skipping escape timeouts.
func (r *Router) HasEscapeSequences() bool {
	return r.hasEscapeSequences
}

// generatesEscapeSequence returns true if the key generates a terminal
// escape sequence (multi-byte starting with ESC).
func generatesEscapeSequence(k Key) bool {
	switch k.Special {
	case SpecialUp, SpecialDown, SpecialLeft, SpecialRight,
		SpecialHome, SpecialEnd, SpecialPageUp, SpecialPageDown,
		SpecialInsert, SpecialDelete,
		SpecialF1, SpecialF2, SpecialF3, SpecialF4, SpecialF5, SpecialF6,
		SpecialF7, SpecialF8, SpecialF9, SpecialF10, SpecialF11, SpecialF12:
		return true
	}
	// Alt+key also generates ESC followed by the key
	if k.Mod&ModAlt != 0 {
		return true
	}
	return false
}

// Timeout sets the timeout for sequence matching.
func (r *Router) Timeout(d time.Duration) *Router {
	r.timeout = d
	return r
}

// Name sets an optional name for the router.
func (r *Router) Name(name string) *Router {
	r.name = name
	return r
}

// GetName returns the router's name.
func (r *Router) GetName() string {
	return r.name
}

// SetAlias defines a pattern alias that expands in Handle patterns.
// Alias names are case-insensitive and use angle bracket syntax.
//
// Example:
//
//	router.SetAlias("Leader", ",")
//	router.SetAlias("Nav", "<C-w>")
//	router.Handle("<Leader>f", ...)  // expands to ",f"
//	router.Handle("<Nav>j", ...)     // expands to "<C-w>j"
func (r *Router) SetAlias(name, expansion string) *Router {
	if r.aliases == nil {
		r.aliases = make(map[string]string)
	}
	r.aliases[strings.ToLower(name)] = expansion
	return r
}

// expandAliases replaces alias references in a pattern with their expansions.
func (r *Router) expandAliases(pattern string) string {
	if r.aliases == nil {
		return pattern
	}

	var result strings.Builder
	i := 0
	for i < len(pattern) {
		if pattern[i] == '<' {
			// Find closing >
			end := strings.IndexByte(pattern[i:], '>')
			if end == -1 {
				result.WriteByte(pattern[i])
				i++
				continue
			}
			end += i

			// Extract the name (without < >)
			name := pattern[i+1 : end]

			// Check if it's an alias (case-insensitive)
			if expansion, ok := r.aliases[strings.ToLower(name)]; ok {
				result.WriteString(expansion)
			} else {
				// Not an alias, keep as-is
				result.WriteString(pattern[i : end+1])
			}
			i = end + 1
		} else {
			result.WriteByte(pattern[i])
			i++
		}
	}
	return result.String()
}

// Handle registers a handler for the given pattern.
//
// Vim-style pattern syntax:
//   - "j"           → single key j
//   - "gg"          → sequence: g then g
//   - "<C-w>"       → Ctrl+W
//   - "<A-x>"       → Alt+X
//   - "<S-Tab>"     → Shift+Tab
//   - "<C-A-d>"     → Ctrl+Alt+D
//   - "<C-w>j"      → Ctrl+W then j
//   - "<C-w><C-j>"  → Ctrl+W then Ctrl+J
//   - "<Esc>"       → Escape key
//   - "<CR>"        → Enter key
//   - "<Space>"     → Space bar
//   - "<F1>"        → F1 key
//   - "<PageUp>"    → Page Up key
func (r *Router) Handle(pattern string, h Handler) {
	r.registerPattern(pattern, h)
}

// HandleNamed registers a handler with a semantic name for introspection and rebinding.
// The name should be a descriptive action like "scroll_down" or "go_to_top".
// Users can later rebind this action using Rebind() or config files.
func (r *Router) HandleNamed(name, defaultPattern string, h Handler) {
	if r.namedBindings == nil {
		r.namedBindings = make(map[string]*namedBinding)
	}

	r.namedBindings[name] = &namedBinding{
		defaultPattern: defaultPattern,
		currentPattern: defaultPattern,
		handler:        h,
	}
	r.bindingOrder = append(r.bindingOrder, name)
	r.registerPattern(defaultPattern, h)
}

// registerPattern does the actual pattern registration in the trie.
func (r *Router) registerPattern(pattern string, h Handler) {
	// Expand any aliases in the pattern
	pattern = r.expandAliases(pattern)

	keys := ParsePattern(pattern)
	if len(keys) == 0 {
		return
	}

	// check if any key in the pattern generates escape sequences
	if slices.ContainsFunc(keys, generatesEscapeSequence) {
		r.hasEscapeSequences = true
	}

	node := r.root
	for _, k := range keys {
		if node.children == nil {
			node.children = make(map[Key]*trieNode)
		}
		child, exists := node.children[k]
		if !exists {
			child = &trieNode{children: make(map[Key]*trieNode)}
			node.children[k] = child
		}
		node = child
	}
	node.handler = h
}

// Rebind changes the pattern for a named binding.
// Returns true if the binding was found and rebound.
func (r *Router) Rebind(name, pattern string) bool {
	binding, ok := r.namedBindings[name]
	if !ok {
		return false
	}

	// Remove old pattern from trie
	r.removePattern(binding.currentPattern)

	// Register new pattern
	binding.currentPattern = pattern
	r.registerPattern(pattern, binding.handler)
	return true
}

// removePattern removes a pattern from the trie.
func (r *Router) removePattern(pattern string) {
	pattern = r.expandAliases(pattern)
	keys := ParsePattern(pattern)
	if len(keys) == 0 {
		return
	}

	// Walk to the node and clear its handler
	node := r.root
	for _, k := range keys {
		child, exists := node.children[k]
		if !exists {
			return
		}
		node = child
	}
	node.handler = nil

	// Note: we don't prune empty branches for simplicity
	// This could be optimized if memory is a concern
}

// Reset restores a named binding to its default pattern.
// Returns true if the binding was found and reset.
func (r *Router) Reset(name string) bool {
	binding, ok := r.namedBindings[name]
	if !ok {
		return false
	}

	if binding.currentPattern == binding.defaultPattern {
		return true // Already at default
	}

	return r.Rebind(name, binding.defaultPattern)
}

// ResetAll restores all named bindings to their defaults.
func (r *Router) ResetAll() {
	for name := range r.namedBindings {
		r.Reset(name)
	}
}

// Bindings returns all named bindings in registration order.
func (r *Router) Bindings() []Binding {
	bindings := make([]Binding, 0, len(r.bindingOrder))
	for _, name := range r.bindingOrder {
		if b, ok := r.namedBindings[name]; ok {
			bindings = append(bindings, Binding{
				Name:           name,
				Pattern:        b.currentPattern,
				DefaultPattern: b.defaultPattern,
			})
		}
	}
	return bindings
}

// BindingsMap returns current bindings as a map (for serialization to config).
func (r *Router) BindingsMap() map[string]string {
	m := make(map[string]string, len(r.namedBindings))
	for name, b := range r.namedBindings {
		m[name] = b.currentPattern
	}
	return m
}

// DefaultBindingsMap returns default bindings as a map.
func (r *Router) DefaultBindingsMap() map[string]string {
	m := make(map[string]string, len(r.namedBindings))
	for name, b := range r.namedBindings {
		m[name] = b.defaultPattern
	}
	return m
}

// ApplyBindings applies a map of name->pattern bindings.
// Unknown names are silently ignored.
func (r *Router) ApplyBindings(bindings map[string]string) {
	for name, pattern := range bindings {
		r.Rebind(name, pattern)
	}
}

// configFile represents the structure of ~/.config/riffkey.toml
type configFile struct {
	Global   map[string]string            `toml:"global"`
	Apps     map[string]map[string]string `toml:"-"` // Filled by custom parsing
	Aliases  map[string]string            `toml:"aliases"`
	rawTable map[string]interface{}
}

// ConfigPath returns the default config file path.
// Respects XDG_CONFIG_HOME if set, otherwise uses ~/.config/riffkey.toml
func ConfigPath() string {
	configDir := os.Getenv("XDG_CONFIG_HOME")
	if configDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		configDir = filepath.Join(home, ".config")
	}
	return filepath.Join(configDir, "riffkey.toml")
}

// LoadBindings loads bindings from the shared config file.
// It merges: defaults → global section → app-specific section.
// Missing file or sections are silently ignored.
func (r *Router) LoadBindings(appName string) error {
	return r.LoadBindingsFrom(ConfigPath(), appName)
}

// LoadBindingsFrom loads bindings from a specific config file.
func (r *Router) LoadBindingsFrom(path, appName string) error {
	if path == "" {
		return nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // Missing config is fine
		}
		return err
	}

	// Parse into a generic map first
	var raw map[string]interface{}
	if _, err := toml.Decode(string(data), &raw); err != nil {
		return err
	}

	// Apply global aliases
	if aliases, ok := raw["aliases"].(map[string]interface{}); ok {
		for name, expansion := range aliases {
			if s, ok := expansion.(string); ok {
				r.SetAlias(name, s)
			}
		}
	}

	// Apply global bindings
	if global, ok := raw["global"].(map[string]interface{}); ok {
		for name, pattern := range global {
			if s, ok := pattern.(string); ok {
				r.Rebind(name, s)
			}
		}
	}

	// Apply app-specific bindings (override global)
	if appSection, ok := raw[appName].(map[string]interface{}); ok {
		for name, pattern := range appSection {
			if s, ok := pattern.(string); ok {
				r.Rebind(name, s)
			}
		}
	}

	return nil
}

// WriteDefaultBindings writes a TOML config template with all bindings commented out.
func (r *Router) WriteDefaultBindings(w io.Writer, appName string) error {
	var sb strings.Builder

	sb.WriteString("[" + appName + "]\n")
	for _, b := range r.Bindings() {
		sb.WriteString("# " + b.Name + " = \"" + b.DefaultPattern + "\"\n")
	}

	_, err := w.Write([]byte(sb.String()))
	return err
}

// match attempts to match a sequence of keys.
func (r *Router) match(keys []Key) (handler Handler, consumed int, partial bool) {
	node := r.root
	var lastHandler Handler
	var lastConsumed int

	for i, k := range keys {
		child, exists := node.children[k]
		if !exists {
			if lastHandler != nil {
				return lastHandler, lastConsumed, false
			}
			return nil, 0, false
		}
		node = child
		if node.handler != nil {
			lastHandler = node.handler
			lastConsumed = i + 1
		}
	}

	partial = len(node.children) > 0
	return lastHandler, lastConsumed, partial
}

// ParsePattern parses a vim-style pattern string into a sequence of Keys.
func ParsePattern(pattern string) []Key {
	if pattern == "" {
		return nil
	}

	var keys []Key
	runes := []rune(pattern)
	i := 0

	for i < len(runes) {
		if runes[i] == '<' {
			// Find closing >
			end := i + 1
			for end < len(runes) && runes[end] != '>' {
				end++
			}
			if end < len(runes) {
				// Parse <...> sequence
				inner := string(runes[i+1 : end])
				key := parseVimKey(inner)
				keys = append(keys, key)
				i = end + 1
				continue
			}
		}
		// Regular character
		keys = append(keys, Key{Rune: runes[i]})
		i++
	}

	return keys
}

// parseVimKey parses the content inside <...>
func parseVimKey(s string) Key {
	var key Key
	parts := strings.Split(s, "-")

	for i, part := range parts {
		lower := strings.ToLower(part)

		// Check for modifiers (only valid before the final part)
		if i < len(parts)-1 {
			switch lower {
			case "c":
				key.Mod |= ModCtrl
				continue
			case "a", "m": // A for Alt, M for Meta (same thing)
				key.Mod |= ModAlt
				continue
			case "s":
				key.Mod |= ModShift
				continue
			}
		}

		// Final part - check if it's a special key
		if special, ok := vimToSpecial[lower]; ok {
			key.Special = special
		} else if len(part) == 1 {
			key.Rune = rune(part[0])
		} else {
			// Unknown, treat first char as the key
			if len(part) > 0 {
				key.Rune = rune(part[0])
			}
		}
	}

	return key
}

// Input manages a stack of routers and dispatches keys.
type Input struct {
	stack       []*Router
	buffer      []Key
	countBuffer string // accumulated digit characters for count prefix
	mu          sync.Mutex
	timer       *time.Timer
	pending     Handler
	pendingKeys []Key
}

// NewInput creates a new Input with the given root router.
func NewInput(root *Router) *Input {
	i := &Input{}
	if root != nil {
		i.stack = []*Router{root}
	}
	return i
}

// Push adds a router to the stack, making it the active router.
func (i *Input) Push(r *Router) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.clearBuffer()
	i.stack = append(i.stack, r)
}

// Pop removes the top router from the stack.
func (i *Input) Pop() {
	i.mu.Lock()
	defer i.mu.Unlock()
	if len(i.stack) > 1 {
		i.clearBuffer()
		i.stack = i.stack[:len(i.stack)-1]
	}
}

// Current returns the currently active router.
func (i *Input) Current() *Router {
	i.mu.Lock()
	defer i.mu.Unlock()
	if len(i.stack) == 0 {
		return nil
	}
	return i.stack[len(i.stack)-1]
}

// Depth returns the current stack depth.
func (i *Input) Depth() int {
	i.mu.Lock()
	defer i.mu.Unlock()
	return len(i.stack)
}

// isDigit checks if the key is a digit (for count prefix).
// Note: 0 is special in vim (start of line), so we don't treat it as count
// when it's the first digit.
func (i *Input) isCountDigit(k Key) bool {
	if k.Mod != ModNone || k.Special != SpecialNone {
		return false
	}
	if len(i.countBuffer) == 0 {
		// First digit: 1-9 only (0 is a command, not a count)
		return k.Rune >= '1' && k.Rune <= '9'
	}
	// Subsequent digits: 0-9
	return k.Rune >= '0' && k.Rune <= '9'
}

// Dispatch processes a key through the current router.
// Returns true if the key was handled.
func (i *Input) Dispatch(key Key) bool {
	i.mu.Lock()
	defer i.mu.Unlock()

	if len(i.stack) == 0 {
		return false
	}

	router := i.stack[len(i.stack)-1]

	// Check if this is a count digit
	if i.isCountDigit(key) && len(i.buffer) == 0 {
		// Accumulate count prefix
		i.countBuffer += string(key.Rune)
		return true
	}

	// Track if we were waiting for more input
	wasPending := i.pending != nil

	// Stop any pending timeout
	if i.timer != nil {
		i.timer.Stop()
		i.timer = nil
	}
	i.pending = nil
	i.pendingKeys = nil

	i.buffer = append(i.buffer, key)

	handler, consumed, partial := router.match(i.buffer)

	// If we were pending and the new key doesn't extend the match AND
	// there's no partial match possible, the sequence is broken
	if wasPending && consumed < len(i.buffer) && !partial {
		i.buffer = nil
		i.countBuffer = ""
		return false
	}

	if handler != nil && !partial {
		// Complete match, no ambiguity - fire immediately
		matchedKeys := make([]Key, consumed)
		copy(matchedKeys, i.buffer[:consumed])
		i.buffer = i.buffer[consumed:]
		count := i.parseCount()
		i.countBuffer = ""

		i.mu.Unlock()
		handler(Match{Keys: matchedKeys, Count: count})
		i.mu.Lock()
		return true
	}

	if handler != nil && partial {
		// We have a match but more input could extend it
		i.pending = handler
		i.pendingKeys = make([]Key, consumed)
		copy(i.pendingKeys, i.buffer[:consumed])
		pendingCount := i.parseCount()

		i.timer = time.AfterFunc(router.timeout, func() {
			i.mu.Lock()
			if i.pending != nil {
				h := i.pending
				keys := i.pendingKeys
				i.pending = nil
				i.pendingKeys = nil
				i.buffer = i.buffer[len(keys):]
				i.countBuffer = ""
				i.mu.Unlock()
				h(Match{Keys: keys, Count: pendingCount})
				return
			}
			i.mu.Unlock()
		})
		return true
	}

	if partial {
		// Partial match, no complete handler yet - wait for more input
		return true
	}

	// No match at all - clear buffer
	i.buffer = nil
	i.countBuffer = ""
	return false
}

// parseCount returns the count prefix, defaulting to 1.
func (i *Input) parseCount() int {
	if i.countBuffer == "" {
		return 1
	}
	n, err := strconv.Atoi(i.countBuffer)
	if err != nil || n < 1 {
		return 1
	}
	return n
}

// clearBuffer resets the input buffer and cancels any pending timeout.
func (i *Input) clearBuffer() {
	if i.timer != nil {
		i.timer.Stop()
		i.timer = nil
	}
	i.pending = nil
	i.pendingKeys = nil
	i.buffer = nil
	i.countBuffer = ""
}

// Flush forces any pending handler to fire immediately.
func (i *Input) Flush() {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.pending != nil {
		h := i.pending
		keys := i.pendingKeys
		count := i.parseCount()
		i.pending = nil
		i.pendingKeys = nil
		i.buffer = nil
		i.countBuffer = ""
		if i.timer != nil {
			i.timer.Stop()
			i.timer = nil
		}
		i.mu.Unlock()
		h(Match{Keys: keys, Count: count})
		i.mu.Lock()
	}
}

// Clear resets the input buffer without firing any handlers.
func (i *Input) Clear() {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.clearBuffer()
}

// Pending returns the current pending key buffer state (for UI display).
func (i *Input) Pending() (count string, keys []Key) {
	i.mu.Lock()
	defer i.mu.Unlock()
	keysCopy := make([]Key, len(i.buffer))
	copy(keysCopy, i.buffer)
	return i.countBuffer, keysCopy
}

// Reader reads terminal input and converts it to Keys.
type Reader struct {
	r       io.Reader
	buf     []byte // internal buffer for unprocessed bytes
	pos     int    // current position in buffer
	end     int    // end of valid data in buffer
	tmp     []byte // temp buffer for reads
	timeout time.Duration

	// For async reading with timeout
	readCh      chan readResult
	readPending bool // true if a goroutine is blocked on Read

	// If false, byte 27 is always Escape (no timeout needed)
	parseEscapeSequences bool
}

type readResult struct {
	n   int
	err error
}

// NewReader creates a Reader that parses terminal input into Keys.
// The timeout is used to distinguish Escape key from escape sequences.
func NewReader(r io.Reader) *Reader {
	return &Reader{
		r:                    r,
		buf:                  make([]byte, 64),
		tmp:                  make([]byte, 32),
		timeout:              50 * time.Millisecond,
		readCh:               make(chan readResult, 1),
		parseEscapeSequences: true, // Default to parsing escape sequences
	}
}

// EscapeTimeout sets the timeout for distinguishing Escape from escape sequences.
func (r *Reader) EscapeTimeout(d time.Duration) *Reader {
	r.timeout = d
	return r
}

// SetParseEscapeSequences configures whether to parse terminal escape sequences.
// If false, byte 27 immediately returns as Escape key (no timeout delay).
// Use router.HasEscapeSequences() to determine if this is needed.
func (r *Reader) SetParseEscapeSequences(parse bool) *Reader {
	r.parseEscapeSequences = parse
	return r
}

// ReadKey reads the next key from the underlying reader.
// It handles escape sequences for special keys (arrows, function keys, etc.).
func (r *Reader) ReadKey() (Key, error) {
	// Ensure we have at least one byte
	if err := r.ensureBytes(1); err != nil {
		return Key{}, err
	}

	// Get first byte
	b := r.buf[r.pos]
	r.pos++

	// Escape sequence - try to get more bytes if needed
	if b == 27 {
		// If not parsing escape sequences, return Escape immediately (no delay)
		if !r.parseEscapeSequences {
			return Key{Special: SpecialEscape}, nil
		}

		// Try to read more bytes for escape sequence (with timeout for TTY)
		r.ensureBytesWithTimeout(1)

		if r.pos < r.end {
			nextByte := r.buf[r.pos]

			// SS3 sequence: ESC O then one more char
			if nextByte == 'O' {
				r.ensureBytesWithTimeout(2) // Try to get the third byte
				if r.pos+1 < r.end {
					seq := []byte{27, 'O', r.buf[r.pos+1]}
					r.pos += 2
					return r.parseBytes(seq), nil
				}
			}

			// CSI sequence: ESC [ ...
			if nextByte == '[' {
				// Try to read enough for the full sequence
				r.ensureBytesWithTimeout(8)
				seqEnd := r.pos + 1 // Start after '['
				for seqEnd < r.end && seqEnd < r.pos+12 {
					c := r.buf[seqEnd]
					seqEnd++
					// CSI terminators: letter or ~
					if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || c == '~' {
						break
					}
				}
				seq := make([]byte, seqEnd-r.pos+1)
				seq[0] = 27
				copy(seq[1:], r.buf[r.pos:seqEnd])
				r.pos = seqEnd
				return r.parseBytes(seq), nil
			}

			// Alt+key: ESC then printable char
			if nextByte >= 32 && nextByte < 127 {
				r.pos++
				return Key{Rune: rune(nextByte), Mod: ModAlt}, nil
			}
		}

		// Just ESC by itself
		return Key{Special: SpecialEscape}, nil
	}

	return r.parseSingleByte(b), nil
}

// ensureBytesWithTimeout is like ensureBytes but uses a timeout for TTY input.
// This allows distinguishing between Escape key and escape sequences.
func (r *Reader) ensureBytesWithTimeout(n int) {
	available := r.end - r.pos
	if available >= n {
		return
	}

	// Shift remaining bytes to start of buffer
	if r.pos > 0 && available > 0 {
		copy(r.buf, r.buf[r.pos:r.end])
		r.end = available
		r.pos = 0
	} else if available == 0 {
		r.pos = 0
		r.end = 0
	}

	// Check for result from a previous pending read first
	if r.readPending {
		select {
		case result := <-r.readCh:
			r.readPending = false
			if result.n > 0 {
				copy(r.buf[r.end:], r.tmp[:result.n])
				r.end += result.n
			}
			if r.end-r.pos >= n {
				return
			}
		default:
			// Still pending, wait with timeout
			select {
			case result := <-r.readCh:
				r.readPending = false
				if result.n > 0 {
					copy(r.buf[r.end:], r.tmp[:result.n])
					r.end += result.n
				}
			case <-time.After(r.timeout):
				// Timeout - read still pending, will get it later
			}
			return
		}
	}

	// Start new async read with timeout
	space := len(r.buf) - r.end
	if space > len(r.tmp) {
		space = len(r.tmp)
	}
	if space > 0 {
		r.readPending = true
		go func() {
			n, err := r.r.Read(r.tmp[:space])
			r.readCh <- readResult{n, err}
		}()

		// Wait for read or timeout
		select {
		case result := <-r.readCh:
			r.readPending = false
			if result.n > 0 {
				copy(r.buf[r.end:], r.tmp[:result.n])
				r.end += result.n
			}
		case <-time.After(r.timeout):
			// Timeout - no more bytes available quickly, so this is likely
			// a standalone Escape, not an escape sequence.
			// readPending stays true, we'll get the result on next read.
		}
	}
}

// ensureBytes tries to ensure at least n bytes are available in the buffer.
// Returns error only if no bytes are available and read fails.
func (r *Reader) ensureBytes(n int) error {
	available := r.end - r.pos
	if available >= n {
		return nil
	}

	// Shift remaining bytes to start of buffer
	if r.pos > 0 && available > 0 {
		copy(r.buf, r.buf[r.pos:r.end])
		r.end = available
		r.pos = 0
	} else if available == 0 {
		r.pos = 0
		r.end = 0
	}

	// If there's a pending read from a previous timeout, wait for it
	if r.readPending {
		result := <-r.readCh
		r.readPending = false
		if result.n > 0 {
			copy(r.buf[r.end:], r.tmp[:result.n])
			r.end += result.n
		}
		if result.err != nil && r.end == r.pos {
			return result.err
		}
		if r.end-r.pos >= n {
			return nil
		}
	}

	// Try to read more
	space := len(r.buf) - r.end
	if space > len(r.tmp) {
		space = len(r.tmp)
	}
	if space > 0 {
		read, err := r.r.Read(r.tmp[:space])
		if read > 0 {
			copy(r.buf[r.end:], r.tmp[:read])
			r.end += read
		}
		if err != nil && r.end == r.pos {
			return err
		}
	}
	return nil
}

// parseBytes converts raw terminal bytes into a Key.
func (r *Reader) parseBytes(b []byte) Key {
	if len(b) == 0 {
		return Key{}
	}

	// Single byte
	if len(b) == 1 {
		return r.parseSingleByte(b[0])
	}

	// Escape sequence
	if b[0] == 27 {
		return r.parseEscapeSequence(b)
	}

	// Multi-byte UTF-8 character
	return Key{Rune: rune(b[0])}
}

// parseSingleByte handles single-byte input.
func (r *Reader) parseSingleByte(b byte) Key {
	switch {
	case b == 27:
		return Key{Special: SpecialEscape}
	case b == 13 || b == 10:
		return Key{Special: SpecialEnter}
	case b == 9:
		return Key{Special: SpecialTab}
	case b == 127 || b == 8:
		return Key{Special: SpecialBackspace}
	case b == 0:
		return Key{Rune: ' ', Mod: ModCtrl} // Ctrl+Space
	case b < 27:
		// Ctrl+A through Ctrl+Z (1-26)
		return Key{Rune: rune('a' + b - 1), Mod: ModCtrl}
	case b == 32:
		return Key{Special: SpecialSpace}
	default:
		return Key{Rune: rune(b)}
	}
}

// parseEscapeSequence handles escape sequences (arrows, function keys, etc.).
func (r *Reader) parseEscapeSequence(b []byte) Key {
	if len(b) == 1 {
		return Key{Special: SpecialEscape}
	}

	// Alt+key: ESC followed by a character
	if len(b) == 2 && b[1] >= 32 && b[1] < 127 {
		return Key{Rune: rune(b[1]), Mod: ModAlt}
	}

	// CSI sequences: ESC [
	if len(b) >= 3 && b[1] == '[' {
		return r.parseCSI(b[2:])
	}

	// SS3 sequences: ESC O (some terminals use this for F1-F4)
	if len(b) >= 3 && b[1] == 'O' {
		return r.parseSS3(b[2:])
	}

	return Key{Special: SpecialEscape}
}

// parseCSI handles CSI (Control Sequence Introducer) sequences: ESC [ ...
func (r *Reader) parseCSI(b []byte) Key {
	if len(b) == 0 {
		return Key{Special: SpecialEscape}
	}

	// Arrow keys and simple sequences
	switch b[0] {
	case 'A':
		return Key{Special: SpecialUp}
	case 'B':
		return Key{Special: SpecialDown}
	case 'C':
		return Key{Special: SpecialRight}
	case 'D':
		return Key{Special: SpecialLeft}
	case 'H':
		return Key{Special: SpecialHome}
	case 'F':
		return Key{Special: SpecialEnd}
	case 'Z':
		return Key{Special: SpecialTab, Mod: ModShift} // Shift+Tab
	}

	// Modified arrows: ESC [ 1 ; mod X
	if len(b) >= 4 && b[0] == '1' && b[1] == ';' {
		mod := r.parseModifier(b[2])
		switch b[3] {
		case 'A':
			return Key{Special: SpecialUp, Mod: mod}
		case 'B':
			return Key{Special: SpecialDown, Mod: mod}
		case 'C':
			return Key{Special: SpecialRight, Mod: mod}
		case 'D':
			return Key{Special: SpecialLeft, Mod: mod}
		case 'H':
			return Key{Special: SpecialHome, Mod: mod}
		case 'F':
			return Key{Special: SpecialEnd, Mod: mod}
		}
	}

	// Tilde sequences: ESC [ N ~ or ESC [ N ; mod ~
	if b[len(b)-1] == '~' {
		return r.parseTildeSequence(b[:len(b)-1])
	}

	return Key{Special: SpecialEscape}
}

// parseTildeSequence handles ESC [ N ~ sequences.
func (r *Reader) parseTildeSequence(b []byte) Key {
	if len(b) == 0 {
		return Key{Special: SpecialEscape}
	}

	// Check for modifier: N ; mod
	var mod Modifier
	numStr := string(b)
	if idx := strings.Index(numStr, ";"); idx != -1 {
		if idx+1 < len(numStr) {
			mod = r.parseModifier(numStr[idx+1])
		}
		numStr = numStr[:idx]
	}

	num, err := strconv.Atoi(numStr)
	if err != nil {
		return Key{Special: SpecialEscape}
	}

	switch num {
	case 1:
		return Key{Special: SpecialHome, Mod: mod}
	case 2:
		return Key{Special: SpecialInsert, Mod: mod}
	case 3:
		return Key{Special: SpecialDelete, Mod: mod}
	case 4:
		return Key{Special: SpecialEnd, Mod: mod}
	case 5:
		return Key{Special: SpecialPageUp, Mod: mod}
	case 6:
		return Key{Special: SpecialPageDown, Mod: mod}
	case 7:
		return Key{Special: SpecialHome, Mod: mod}
	case 8:
		return Key{Special: SpecialEnd, Mod: mod}
	case 11, 12, 13, 14, 15:
		return Key{Special: Special(SpecialF1 + Special(num-11)), Mod: mod}
	case 17, 18, 19, 20, 21:
		return Key{Special: Special(SpecialF6 + Special(num-17)), Mod: mod}
	case 23, 24:
		return Key{Special: Special(SpecialF11 + Special(num-23)), Mod: mod}
	}

	return Key{Special: SpecialEscape}
}

// parseSS3 handles SS3 sequences: ESC O ...
func (r *Reader) parseSS3(b []byte) Key {
	if len(b) == 0 {
		return Key{Special: SpecialEscape}
	}

	switch b[0] {
	case 'P':
		return Key{Special: SpecialF1}
	case 'Q':
		return Key{Special: SpecialF2}
	case 'R':
		return Key{Special: SpecialF3}
	case 'S':
		return Key{Special: SpecialF4}
	case 'H':
		return Key{Special: SpecialHome}
	case 'F':
		return Key{Special: SpecialEnd}
	case 'A':
		return Key{Special: SpecialUp}
	case 'B':
		return Key{Special: SpecialDown}
	case 'C':
		return Key{Special: SpecialRight}
	case 'D':
		return Key{Special: SpecialLeft}
	}

	return Key{Special: SpecialEscape}
}

// parseModifier converts a modifier number to Modifier flags.
// Terminal modifier encoding: 1 + (shift?1:0) + (alt?2:0) + (ctrl?4:0)
func (r *Reader) parseModifier(b byte) Modifier {
	n := int(b - '1')
	var mod Modifier
	if n&1 != 0 {
		mod |= ModShift
	}
	if n&2 != 0 {
		mod |= ModAlt
	}
	if n&4 != 0 {
		mod |= ModCtrl
	}
	return mod
}

// Run reads keys from the reader and dispatches them to the Input.
// It blocks until the reader returns an error (including io.EOF).
// The callback is called after each dispatch for rendering/updates.
// It automatically configures the reader based on the router's requirements.
func (i *Input) Run(r *Reader, afterDispatch func(handled bool)) error {
	// Auto-configure reader based on router's escape sequence requirements
	i.mu.Lock()
	if len(i.stack) > 0 {
		r.SetParseEscapeSequences(i.stack[len(i.stack)-1].HasEscapeSequences())
	}
	i.mu.Unlock()

	for {
		key, err := r.ReadKey()
		if err != nil {
			return err
		}
		handled := i.Dispatch(key)
		if afterDispatch != nil {
			afterDispatch(handled)
		}
	}
}
