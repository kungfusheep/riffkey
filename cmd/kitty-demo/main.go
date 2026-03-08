package main

import (
	"fmt"
	"os"

	"github.com/kungfusheep/riffkey"
	"golang.org/x/term"
)

func main() {
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to set raw mode: %v\n", err)
		os.Exit(1)
	}
	defer term.Restore(int(os.Stdin.Fd()), oldState)
	// clear any stale kitty protocol state from previous runs, then ensure clean exit
	os.Stdout.WriteString("\x1b[<99u")
	defer os.Stdout.WriteString("\x1b[<u")

	write := func(s string) { os.Stdout.WriteString(s) }
	writeln := func(s string) { write(s + "\r\n") }

	kittyEnabled := false
	reader := riffkey.NewReader(os.Stdin)

	writeln("== riffkey kitty keyboard protocol demo ==")
	writeln("")
	writeln("  k - toggle kitty protocol on/off")
	writeln("  q - quit")
	writeln("")
	writeln("[legacy mode] try: Enter vs Ctrl+M, Tab vs Ctrl+I")
	writeln("")

	for {
		key, err := reader.ReadKey()
		if err != nil {
			writeln(fmt.Sprintf("error: %v", err))
			break
		}

		if key.Rune == 'k' && key.Mod == riffkey.ModNone {
			kittyEnabled = !kittyEnabled
			if kittyEnabled {
				write(riffkey.KittyKeyboardEnable)
				writeln("[kitty mode ON]")
			} else {
				write(riffkey.KittyKeyboardDisable)
				writeln("[kitty mode OFF]")
			}
			continue
		}

		if key.Rune == 'q' && key.Mod == riffkey.ModNone {
			if kittyEnabled {
				write(riffkey.KittyKeyboardDisable)
			}
			writeln("bye!")
			break
		}

		mode := "legacy"
		if kittyEnabled {
			mode = "kitty"
		}
		writeln(fmt.Sprintf("[%s] %s", mode, key))
	}
}
