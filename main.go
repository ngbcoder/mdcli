package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"
)

const (
	defaultWidth = 80
	helpText     = "↑/k up • ↓/j down • pgup/pgdn • q quit"
)

func terminalWidth(fallback int) int {
	w, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || w <= 0 {
		return fallback
	}
	return w
}

func terminalHeight(fallback int) int {
	_, h, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || h <= 0 {
		return fallback
	}
	return h
}

func renderMarkdown(content []byte, width int) (string, error) {
	r, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return "", err
	}
	out, err := r.RenderBytes(content)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// ---- plain mode (original behaviour) ----

func runPlain(paths []string) int {
	width := terminalWidth(defaultWidth)
	failed := false
	printed := false

	for _, path := range paths {
		content, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "mdcli: %v\n", err)
			failed = true
			continue
		}

		out, err := renderMarkdown(content, width)
		if err != nil {
			fmt.Fprintf(os.Stderr, "mdcli: %s: %v\n", path, err)
			failed = true
			continue
		}

		if printed && len(paths) > 1 {
			fmt.Println()
		}
		os.Stdout.WriteString(out)
		printed = true
	}

	if failed {
		return 1
	}
	return 0
}

// ---- TUI mode ----

type model struct {
	viewport viewport.Model
	ready    bool
	content  string
	title    string
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return m, tea.Quit
		}
	case tea.WindowSizeMsg:
		headerHeight := lipgloss.Height(m.headerView())
		footerHeight := lipgloss.Height(m.footerView())
		verticalMargin := headerHeight + footerHeight

		if !m.ready {
			m.viewport = viewport.New(msg.Width, msg.Height-verticalMargin)
			m.viewport.YPosition = headerHeight
			m.viewport.SetContent(m.content)
			m.ready = true
		} else {
			m.viewport.Width = msg.Width
			m.viewport.Height = msg.Height - verticalMargin
		}
	}

	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)
	return m, tea.Batch(cmds...)
}

func (m model) headerView() string {
	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("62")).
		Render(m.title)
	line := strings.Repeat("─", max(0, m.viewport.Width-lipgloss.Width(title)-1))
	return lipgloss.JoinHorizontal(lipgloss.Center, title, " ", line)
}

func (m model) footerView() string {
	info := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		Render(fmt.Sprintf("%3.f%%", m.viewport.ScrollPercent()*100))
	line := strings.Repeat("─", max(0, m.viewport.Width-lipgloss.Width(info)-1))
	help := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		Render(helpText)
	return lipgloss.JoinVertical(lipgloss.Left,
		lipgloss.JoinHorizontal(lipgloss.Center, line, " ", info),
		help,
	)
}

func (m model) View() string {
	if !m.ready {
		return "\n  Initializing..."
	}
	return fmt.Sprintf("%s\n%s\n%s", m.headerView(), m.viewport.View(), m.footerView())
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func runTUI(paths []string) int {
	if len(paths) == 0 {
		fmt.Fprintln(os.Stderr, "mdcli: no files given")
		return 1
	}

	// For simplicity, show the first file in TUI (or concatenate with separators)
	var b strings.Builder
	titles := make([]string, 0, len(paths))

	width := terminalWidth(defaultWidth)

	for i, path := range paths {
		content, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "mdcli: %v\n", err)
			return 1
		}

		out, err := renderMarkdown(content, width)
		if err != nil {
			fmt.Fprintf(os.Stderr, "mdcli: %s: %v\n", path, err)
			return 1
		}

		if i > 0 {
			b.WriteString("\n\n")
			b.WriteString(strings.Repeat("═", width))
			b.WriteString("\n\n")
		}
		b.WriteString(out)
		titles = append(titles, path)
	}

	title := strings.Join(titles, " · ")
	if len(title) > 40 {
		title = titles[0]
		if len(paths) > 1 {
			title += fmt.Sprintf(" (+%d more)", len(paths)-1)
		}
	}

	m := model{
		content: b.String(),
		title:   title,
	}

	p := tea.NewProgram(
		m,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "mdcli: %v\n", err)
		return 1
	}
	return 0
}

func main() {
	tui := flag.Bool("tui", false, "open an interactive TUI viewer")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: mdcli [flags] <file.md> [more.md ...]\n\n")
		fmt.Fprintf(os.Stderr, "Render Markdown files in the terminal.\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		flag.Usage()
		os.Exit(1)
	}

	var code int
	if *tui {
		code = runTUI(args)
	} else {
		code = runPlain(args)
	}
	os.Exit(code)
}