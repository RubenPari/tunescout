// Package tui implementa la modalità interattiva di discover.
package tui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/rubenpari/tunescout/internal/store"
)

// Actions sono le operazioni eseguite dalla TUI (implementate in cmd).
type Actions interface {
	Like(ctx context.Context, s store.Suggestion) error
	Skip(ctx context.Context, s store.Suggestion) error
	AddToPlaylist(ctx context.Context, s store.Suggestion) (playlist string, err error)
	Open(s store.Suggestion)
}

var (
	titleStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("10"))
	cursorStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("13"))
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	reasonStyle = lipgloss.NewStyle().Italic(true).Foreground(lipgloss.Color("12"))
	likeStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	skipStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	errStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
)

type item struct {
	s      store.Suggestion
	state  string // "", "like", "skip"
	inList bool
}

type model struct {
	ctx    context.Context
	act    Actions
	items  []item
	cursor int
	status string
	isErr  bool
	busy   bool
	height int
}

type doneMsg struct {
	idx    int
	state  string
	inList bool
	status string
	err    error
}

func Run(ctx context.Context, list []store.Suggestion, act Actions) error {
	m := model{ctx: ctx, act: act, height: 24}
	for _, s := range list {
		m.items = append(m.items, item{s: s})
	}
	_, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	return err
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.height = msg.Height
	case doneMsg:
		m.busy = false
		if msg.err != nil {
			m.status, m.isErr = msg.err.Error(), true
			return m, nil
		}
		it := &m.items[msg.idx]
		if msg.state != "" {
			it.state = msg.state
		}
		it.inList = it.inList || msg.inList
		m.status, m.isErr = msg.status, false
		if msg.state != "" && m.cursor < len(m.items)-1 {
			m.cursor++
		}
	case tea.KeyMsg:
		if m.busy && msg.String() != "ctrl+c" {
			return m, nil
		}
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.items)-1 {
				m.cursor++
			}
		case "l", "enter":
			return m.run(func(s store.Suggestion) doneMsg {
				err := m.act.Like(m.ctx, s)
				return doneMsg{state: "like", status: "♥ salvato nella libreria: " + s.Title, err: err}
			})
		case "s":
			return m.run(func(s store.Suggestion) doneMsg {
				err := m.act.Skip(m.ctx, s)
				return doneMsg{state: "skip", status: "✗ scartato: " + s.Title, err: err}
			})
		case "a":
			return m.run(func(s store.Suggestion) doneMsg {
				pl, err := m.act.AddToPlaylist(m.ctx, s)
				return doneMsg{inList: true, status: fmt.Sprintf("+ aggiunto alla playlist %q", pl), err: err}
			})
		case "o":
			if len(m.items) > 0 {
				m.act.Open(m.items[m.cursor].s)
				m.status, m.isErr = "Apro in Spotify…", false
			}
		}
	}
	return m, nil
}

func (m model) run(f func(store.Suggestion) doneMsg) (tea.Model, tea.Cmd) {
	if len(m.items) == 0 {
		return m, nil
	}
	idx := m.cursor
	s := m.items[idx].s
	m.busy, m.status, m.isErr = true, "…", false
	return m, func() tea.Msg {
		d := f(s)
		d.idx = idx
		return d
	}
}

func (m model) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("tunescout · nuove scoperte") + "\n\n")
	if len(m.items) == 0 {
		b.WriteString("Nessun suggerimento.\n")
	}

	// ogni item occupa 3 righe; scrolla per tenere visibile il cursore
	perPage := max(1, (m.height-6)/3)
	start := 0
	if m.cursor >= perPage {
		start = m.cursor - perPage + 1
	}
	end := min(len(m.items), start+perPage)

	for i := start; i < end; i++ {
		it := m.items[i]
		mark := "  "
		line := fmt.Sprintf("%s — %s", it.s.Artist, it.s.Title)
		if i == m.cursor {
			mark = cursorStyle.Render("▶ ")
			line = cursorStyle.Render(line)
		}
		switch it.state {
		case "like":
			line += likeStyle.Render("  ♥")
		case "skip":
			line += skipStyle.Render("  ✗")
		}
		if it.inList {
			line += dimStyle.Render("  [playlist]")
		}
		meta := it.s.Album
		if it.s.Year != "" {
			meta += " (" + it.s.Year + ")"
		}
		fmt.Fprintf(&b, "%s%s\n   %s\n   %s\n", mark, line, dimStyle.Render(meta), reasonStyle.Render(it.s.Reason))
	}

	b.WriteString("\n")
	if m.status != "" {
		if m.isErr {
			b.WriteString(errStyle.Render(m.status) + "\n")
		} else {
			b.WriteString(m.status + "\n")
		}
	}
	b.WriteString(dimStyle.Render(fmt.Sprintf("%d/%d · ↑↓ naviga · l like (salva) · s skip · a aggiungi a playlist · o apri · q esci",
		m.cursor+1, len(m.items))))
	return b.String()
}
