package main

import (
	"fmt"
	"os"

	"golang.org/x/term"
)

// run with: go run ./cmd/kitty-demo/raw_test.go
// tests whether the terminal responds to kitty keyboard protocol
func main() {
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed: %v\n", err)
		os.Exit(1)
	}
	defer term.Restore(int(os.Stdin.Fd()), oldState)
	defer os.Stdout.WriteString("\x1b[<u") // always pop kitty protocol on exit

	w := func(s string) { os.Stdout.WriteString(s + "\r\n") }

	// query current kitty flags
	w("querying kitty keyboard support...")
	os.Stdout.WriteString("\x1b[?u")
	// also request primary device attributes as a sentinel
	os.Stdout.WriteString("\x1b[c")

	w("reading response bytes (press any key if nothing appears):")
	buf := make([]byte, 64)
	n, _ := os.Stdin.Read(buf)
	s := fmt.Sprintf("  response [%d bytes]:", n)
	for i := 0; i < n; i++ {
		s += fmt.Sprintf(" %02x", buf[i])
	}
	s += "  |"
	for i := 0; i < n; i++ {
		if buf[i] >= 32 && buf[i] < 127 {
			s += string(buf[i])
		} else {
			s += "."
		}
	}
	s += "|"
	w(s)
	w("")

	// now enable kitty protocol and test
	w("enabling kitty protocol with ESC[>1u ...")
	os.Stdout.WriteString("\x1b[>1u")

	w("press Enter, then Ctrl+M, then 'q' to quit:")
	w("")
	for {
		n, err := os.Stdin.Read(buf)
		if err != nil {
			break
		}
		hex := fmt.Sprintf("  [%d bytes]:", n)
		for i := 0; i < n; i++ {
			hex += fmt.Sprintf(" %02x", buf[i])
		}
		hex += "  |"
		for i := 0; i < n; i++ {
			if buf[i] >= 32 && buf[i] < 127 {
				hex += string(buf[i])
			} else {
				hex += "."
			}
		}
		hex += "|"
		w(hex)

		// quit on 'q'
		if n == 1 && buf[0] == 'q' {
			os.Stdout.WriteString("\x1b[<u") // pop kitty
			w("bye!")
			break
		}
	}
}
