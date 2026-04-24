# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Test Commands

```bash
# Build
go build ./...

# Run all tests
go test ./...

# Run a specific test
go test -run TestParsePattern ./...

# Run tests with verbose output
go test -v ./...

# Run tests with race detector
go test -race ./...
```

## Project Overview

**riffkey** is a terminal key input router library for Go that provides vim-style pattern matching and shared configuration. It enables building terminal UIs with sophisticated keybinding support similar to vim, neovim, and other modal editors.

## Architecture

### Core Types

- **Key** - Represents a single keypress with optional modifiers (Ctrl, Alt, Shift) and special key types (F-keys, arrows, etc.)
- **Router** - Matches key patterns to handlers using a trie data structure; supports named bindings, aliases, and TOML config loading
- **Input** - Manages a stack of routers for modal input (normal/insert/visual modes) and handles dispatch with count prefixes
- **Reader** - Parses raw terminal bytes into Key structs, handling escape sequences with timeout-based disambiguation

### Key Design Patterns

1. **Trie-based pattern matching** - Patterns like `gg`, `<C-w>j`, `diw` are stored in a trie for efficient prefix matching
2. **Timeout-based disambiguation** - When patterns overlap (e.g., `g` vs `gg`), the router waits for a configurable timeout before firing the shorter match
3. **Router stack** - Push/pop routers for modal input. Dispatch walks every enabled router in the top frame, so panes and other in-context bindings can attach without a modal push.
4. **Named bindings** - Handlers registered with `HandleNamed()` can be introspected and rebound at runtime
5. **Shared config** - TOML config at `~/.config/riffkey.toml` with `[global]` and per-app sections

### Pattern Syntax

Vim-style patterns: `j`, `gg`, `<C-w>`, `<A-x>`, `<S-Tab>`, `<CR>`, `<Up>`, `<F1>`, etc. Aliases expand once (no recursion).

### Concurrency

The Input type uses a mutex for thread-safe dispatch. Handlers may be called from timer goroutines (for timeout-based disambiguation).
