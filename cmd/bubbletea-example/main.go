// Example: Using riffkey with Bubble Tea via HandleMsg
//
// Run with: go run main.go
package main

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/kungfusheep/riffkey"
	"golang.org/x/term"
)

type moveCmd int
type deleteCmd struct{}

func main() {
	oldState, _ := term.MakeRaw(int(os.Stdin.Fd()))
	defer term.Restore(int(os.Stdin.Fd()), oldState)

	p := tea.NewProgram(newModel(), tea.WithInput(nil), tea.WithAltScreen())

	router := riffkey.NewRouter(riffkey.WithSender(p))

	router.HandleNamedMsg("move_down", "j", func(m riffkey.Match) any { return moveCmd(m.Count) })
	router.HandleNamedMsg("move_up", "k", func(m riffkey.Match) any { return moveCmd(-m.Count) })
	router.HandleNamedMsg("top", "gg", func(m riffkey.Match) any { return moveCmd(-1000) })
	router.HandleNamedMsg("bottom", "G", func(m riffkey.Match) any { return moveCmd(1000) })
	router.HandleNamedMsg("delete", "dd", func(m riffkey.Match) any { return deleteCmd{} })
	router.HandleNamedMsg("quit", "q", func(m riffkey.Match) any { return tea.Quit() })

	router.LoadBindings("bbt_example")

	go riffkey.NewInput(router).Run(riffkey.NewReader(os.Stdin), nil)

	p.Run()
}

type model struct {
	items  []string
	cursor int
}

func newModel() model {
	return model{items: []string{
		"Learn riffkey", "Build a TUI", "Add vim bindings",
		"Count prefixes (5j)", "Multi-key sequences (gg, dd)", "Profit!",
	}}
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case moveCmd:
		m.cursor = max(0, min(m.cursor+int(msg), len(m.items)-1))
	case deleteCmd:
		if len(m.items) > 0 {
			m.items = append(m.items[:m.cursor], m.items[m.cursor+1:]...)
			m.cursor = min(m.cursor, len(m.items)-1)
		}
	case tea.QuitMsg:
		return m, tea.Quit
	}
	return m, nil
}

func (m model) View() string {
	var sb strings.Builder
	sb.WriteString("\n  riffkey + Bubble Tea\n  ────────────────────\n\n")
	for i, item := range m.items {
		cursor := "   "
		if i == m.cursor {
			cursor = " ▸ "
		}
		sb.WriteString(fmt.Sprintf("%s%s\n", cursor, item))
	}
	sb.WriteString("\n  j/k: move  5j: move 5  gg/G: ends  dd: delete  q: quit\n")
	return sb.String()
}
