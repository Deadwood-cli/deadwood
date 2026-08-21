// Package tui is the interactive checklist for `deadwood clean`.
// It has no git or GitHub dependency: the cli layer hands it classified items.
package tui

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Deadwood-cli/deadwood/internal/classify"
	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/term"
)

// Item is one branch offered for deletion.
type Item struct {
	Name    string
	Bucket  classify.Bucket
	Reason  string
	Checked bool
}

// Model is the bubbletea checklist. Cursor moves across branches only;
// bucket headers are labels, not selectable rows.
type Model struct {
	items     []Item
	cursor    int
	submitted bool
	cancelled bool
}

// NewModel builds a checklist from items. The caller's Checked flags are the
// initial selection (safe/squash pre-checked; needs-review only with --include-needs-review).
func NewModel(items []Item) Model {
	return Model{items: append([]Item(nil), items...)}
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd { return nil }

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if len(m.items) == 0 {
		if _, ok := msg.(tea.KeyMsg); ok {
			m.cancelled = true
			return m, tea.Quit
		}
		return m, nil
	}

	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	switch key.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.items)-1 {
			m.cursor++
		}
	case " ", "space":
		m.items[m.cursor].Checked = !m.items[m.cursor].Checked
	case "a":
		m.setBucket(m.items[m.cursor].Bucket, true)
	case "n":
		m.setBucket(m.items[m.cursor].Bucket, false)
	case "enter":
		m.submitted = true
		return m, tea.Quit
	case "q", "esc", "ctrl+c":
		m.cancelled = true
		return m, tea.Quit
	}
	return m, nil
}

func (m *Model) setBucket(bucket classify.Bucket, checked bool) {
	for i := range m.items {
		if m.items[i].Bucket == bucket {
			m.items[i].Checked = checked
		}
	}
}

// View implements tea.Model.
func (m Model) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Select branches to delete"))
	b.WriteString("\n\n")

	var last classify.Bucket
	for i, item := range m.items {
		if item.Bucket != last {
			if last != "" {
				b.WriteString("\n")
			}
			b.WriteString(sectionStyle.Render(bucketLabel(item.Bucket)))
			b.WriteString("\n")
			last = item.Bucket
		}

		mark := "[ ]"
		if item.Checked {
			mark = "[x]"
		}
		cursor := "  "
		line := fmt.Sprintf("%s %s %s", cursor, mark, item.Name)
		if item.Reason != "" {
			line += "  " + dimStyle.Render(item.Reason)
		}
		if i == m.cursor {
			line = cursorStyle.Render("> "+mark+" "+item.Name) + "  " + dimStyle.Render(item.Reason)
		}
		b.WriteString(line)
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(helpStyle.Render("↑/↓ move  space toggle  a all  n none  enter confirm  q/esc cancel"))
	b.WriteString("\n")
	return b.String()
}

// Submitted is true when the user confirmed the current checks with enter.
func (m Model) Submitted() bool { return m.submitted }

// Cancelled is true when the user quit without confirming.
func (m Model) Cancelled() bool { return m.cancelled }

// Selected returns the checked items, in list order.
func (m Model) Selected() []Item {
	out := make([]Item, 0, len(m.items))
	for _, item := range m.items {
		if item.Checked {
			out = append(out, item)
		}
	}
	return out
}

// CheckedItems returns items whose Checked flag is set. Used when the
// checklist is skipped because stdin is not a terminal.
func CheckedItems(items []Item) []Item {
	m := NewModel(items)
	return m.Selected()
}

// Run launches the interactive checklist. If in is not a terminal, it returns
// the pre-checked items without drawing a TUI so scripting and tests cannot hang.
func Run(items []Item, in io.Reader, out io.Writer) (selected []Item, cancelled bool, err error) {
	if !isTerminal(in) {
		return CheckedItems(items), false, nil
	}

	prog := tea.NewProgram(NewModel(items), tea.WithInput(in), tea.WithOutput(out), tea.WithAltScreen())
	final, err := prog.Run()
	if err != nil {
		return nil, false, err
	}
	model, ok := final.(Model)
	if !ok {
		return nil, false, fmt.Errorf("unexpected checklist model type %T", final)
	}
	if model.Cancelled() {
		return nil, true, nil
	}
	if !model.Submitted() {
		return nil, true, nil
	}
	return model.Selected(), false, nil
}

// InputIsTerminal reports whether in is an interactive terminal.
func InputIsTerminal(in io.Reader) bool {
	return isTerminal(in)
}

func isTerminal(in io.Reader) bool {
	f, ok := in.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(f.Fd()))
}

func bucketLabel(bucket classify.Bucket) string {
	switch bucket {
	case classify.BucketSafeDelete:
		return "Safe to delete"
	case classify.BucketSquashMerged:
		return "Squash-merged"
	case classify.BucketNeedsReview:
		return "Needs review"
	default:
		return string(bucket)
	}
}
