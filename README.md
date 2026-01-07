# riffkey

Terminal key input router for Go with vim-esque pattern matching and shared configuration.

## Features

- Handler pattern with sequences (`gg`, `<C-w>j`, `<Leader>f`)
- Count prefixes (`5j` → `m.Count = 5`)
- Push/pop router mechanics for easy modal input
- Hooks (before/after handlers)
- Router cloning for mode-specific behavior
- Macro recording and playback
- Named bindings with runtime rebinding
- Custom aliases e.g. `<Leader>`
- Timeout-based disambiguation for overlapping patterns
- Optional shared config via `~/.config/riffkey.toml`
- Easy Bubble Tea helpers

## Usage

```go
router := riffkey.NewRouter()

router.Handle("j", func(m riffkey.Match) {
    scroll(m.Count)  // m.Count defaults to 1, or the prefix if given (e.g., 5j)
})
router.Handle("gg", func(m riffkey.Match) { goToTop() })
router.Handle("<C-d>", func(m riffkey.Match) { halfPageDown() })
router.Handle("<Up>", func(m riffkey.Match) { cursorUp() })

input := riffkey.NewInput(router)
reader := riffkey.NewReader(os.Stdin)

input.Run(reader, func(handled bool) {
    redraw()
})
```

## Pattern Syntax

Patterns are case-sensitive.

| Pattern | Description |
|---------|-------------|
| `j` | Lowercase j |
| `J` | Uppercase J (distinct from j) |
| `gg` | Sequence: g then g |
| `gj` | Sequence: g then j |
| `G` | Uppercase G |
| `ZZ` | Sequence: Z then Z |
| `<C-w>` | Ctrl+w |
| `<C-W>` | Ctrl+W (modifiers are case-insensitive) |
| `<A-x>` | Alt+x |
| `<M-x>` | Alt+x (M is alias for Alt) |
| `<S-Tab>` | Shift+Tab |
| `<C-A-d>` | Ctrl+Alt+d |
| `<C-w><C-j>` | Ctrl+w then Ctrl+j |
| `<C-w>j` | Ctrl+w then j |
| `<Esc>` | Escape |
| `<CR>` or `<Enter>` | Enter |
| `<Space>` | Space |
| `<BS>` or `<Backspace>` | Backspace |
| `<Tab>` | Tab |
| `<Up>` `<Down>` `<Left>` `<Right>` | Arrow keys |
| `<PageUp>` `<PageDown>` | Page navigation |
| `<Home>` `<End>` | Line navigation |
| `<Insert>` `<Delete>` | Insert/Delete |
| `<F1>` - `<F12>` | Function keys |

## Aliases

Define custom aliases that expand in patterns:

```go
router := riffkey.NewRouter().
    SetAlias("Leader", ",").
    SetAlias("Nav", "<C-w>")

router.Handle("<Leader>f", func(m riffkey.Match) { findFiles() })
router.Handle("<Nav>j", func(m riffkey.Match) { windowDown() })
```

Alias names are case-insensitive. Expansion happens once (no recursive expansion).

## Named Bindings

Register handlers with semantic names for introspection and user configuration:

```go
router := riffkey.NewRouter()

router.HandleNamed("scroll_down", "j", scrollDown)
router.HandleNamed("scroll_up", "k", scrollUp)
router.HandleNamed("go_to_top", "gg", goToTop)
router.HandleNamed("window_down", "<C-w>j", windowDown)

// Programmatically rebind
router.Rebind("scroll_down", "n")

// List all bindings (for help screens)
for _, b := range router.Bindings() {
    fmt.Printf("%-20s %s\n", b.Name, b.Pattern)
}

// Reset to defaults
router.Reset("scroll_down")
router.ResetAll()
```

## Shared Configuration

Load bindings from `~/.config/riffkey.toml`:

```go
router := riffkey.NewRouter()
router.HandleNamed("scroll_down", "j", scrollDown)
router.HandleNamed("quit", "q", quit)

// Loads: defaults -> [global] -> [appname]
router.LoadBindings("browse")
```

Config file format:

```toml
# Global bindings (shared across all apps)
[global]
scroll_down = "j"
scroll_up = "k"
quit = "q"

# App-specific overrides
[browse]
follow_link = "f"
preview_link = "K"

[lazygit]
quit = "Q"

# Shared aliases
[aliases]
Leader = ","
Nav = "<C-w>"
```

Generate a config template:

```go
router.WriteDefaultBindings(os.Stdout, "myapp")
// Output:
// [myapp]
// # scroll_down = "j"
// # scroll_up = "k"
```

## Count Prefixes

Vim-style count prefixes are first-class:

```go
router.Handle("j", func(m riffkey.Match) {
    for i := 0; i < m.Count; i++ {
        moveDown()
    }
})
// 5j calls handler with m.Count = 5
// j calls handler with m.Count = 1
```

Note: `0` is a command, not a count prefix (vim behavior).

## Router Stack

Push/pop routers for modal input:

```go
input := riffkey.NewInput(normalRouter)

// Enter insert mode
input.Push(insertRouter)

// Back to normal mode
input.Pop()
```

## Hooks

Register callbacks that run before or after every matched handler:

```go
// Clone with hook (creates new router sharing handlers)
visualRouter := normalRouter.Clone().OnAfter(func() {
    refreshSelection()
})

// Or add hook in-place to existing router
router.AddOnAfter(func() {
    updateDisplay()
    updateCursor()
})
```

This is useful for mode-specific behavior. For example, in a vim-like editor:
- Normal mode handlers update the display after each command
- Visual mode handlers refresh selection highlighting instead

```go
// Normal mode - display updates after each handler
normalRouter.AddOnAfter(func() {
    ed.updateDisplay()
    ed.updateCursor()
})

// Visual mode - clone handlers, different post-processing
visualRouter := normalRouter.Clone().OnAfter(func() {
    ed.refreshSelection()
})
```

Methods:
- `Clone()` - shallow copy sharing handlers, fresh hooks
- `OnBefore(fn)` / `OnAfter(fn)` - clone with hook added
- `AddOnBefore(fn)` / `AddOnAfter(fn)` - add hook in-place

## Macros

Record and playback key sequences:

```go
input := riffkey.NewInput(router)

// Start recording
input.StartRecording()

// ... keys dispatched here are captured ...

// Stop and get the macro (last key auto-excluded)
macro := input.StopRecording()

// Play it back
input.ExecuteMacro(macro)

// Check recording state (for UI feedback)
if input.IsRecording() {
    statusLine = "Recording..."
}
```

riffkey handles the mechanics - your app manages storage:

```go
var savedMacro riffkey.Macro

router.Handle("q", func(m riffkey.Match) {
    if input.IsRecording() {
        savedMacro = input.StopRecording()
    } else {
        input.StartRecording()
    }
})

router.Handle("@", func(m riffkey.Match) {
    input.ExecuteMacro(savedMacro)
})
```

Keys dispatched during `ExecuteMacro` are not recorded, preventing nested recording loops.

## Ambiguous Sequences

When patterns overlap (e.g., `g` and `gg`), the router waits for the timeout before firing the shorter match:

```go
router := riffkey.NewRouter().Timeout(2 * time.Second)
router.Handle("g", func(m riffkey.Match) { /* ... */ })
router.Handle("gg", func(m riffkey.Match) { /* ... */ })
// Pressing "gg" fires gg immediately
// Pressing "g" then waiting fires g after timeout
// Pressing "g" then "x" cancels g and processes x
```

## Escape Key Handling

The reader automatically detects whether the router uses escape sequences (arrow keys, F-keys, Alt+key). If not, the Escape key returns immediately without the 50ms detection delay.

## Bubble Tea Integration

riffkey offers an alternative to Bubble Tea's input handling, providing vim-style sequences, count prefixes, and shared config.

```go
p := tea.NewProgram(model, tea.WithInput(nil), tea.WithAltScreen())

router := riffkey.NewRouter(riffkey.WithSender(p))

router.HandleNamedMsg("move_down", "j", func(m riffkey.Match) any {
    return moveCmd(m.Count)
})
router.HandleNamedMsg("window_down", "<C-w>j", func(m riffkey.Match) any {
    return focusPaneDown{}
})
router.HandleNamedMsg("quit", "q", func(m riffkey.Match) any {
    return tea.Quit()
})

router.LoadBindings("myapp")

go riffkey.NewInput(router).Run(riffkey.NewReader(os.Stdin), nil)

p.Run()
```

`HandleMsg` and `HandleNamedMsg` return messages that are passed to `Send`. The generic `WithSender[T]` works with any type that has a `Send(T)` method.

See [cmd/bubbletea-example](cmd/bubbletea-example/main.go) for a complete working example.
