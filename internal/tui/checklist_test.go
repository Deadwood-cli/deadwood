package tui

import (
	"strings"
	"testing"

	"github.com/Deadwood-cli/deadwood/internal/classify"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sampleItems() []Item {
	return []Item{
		{Name: "safe-a", Bucket: classify.BucketSafeDelete, Reason: "merged", Checked: true},
		{Name: "safe-b", Bucket: classify.BucketSafeDelete, Reason: "merged", Checked: true},
		{Name: "squash-a", Bucket: classify.BucketSquashMerged, Reason: "PR #1", Checked: true},
		{Name: "review-a", Bucket: classify.BucketNeedsReview, Reason: "ahead", Checked: false},
	}
}

func applyKey(t *testing.T, m Model, key string) Model {
	t.Helper()
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
	out, ok := next.(Model)
	require.True(t, ok)
	return out
}

func TestSpaceTogglesCurrentItem(t *testing.T) {
	m := NewModel(sampleItems())
	m = applyKey(t, m, " ")
	assert.False(t, m.items[0].Checked)

	m = applyKey(t, m, " ")
	assert.True(t, m.items[0].Checked)
}

func TestASelectsAllInCurrentBucket(t *testing.T) {
	m := NewModel(sampleItems())
	m.items[0].Checked = false
	m.items[1].Checked = false
	m = applyKey(t, m, "a")

	assert.True(t, m.items[0].Checked)
	assert.True(t, m.items[1].Checked)
	assert.True(t, m.items[2].Checked, "other buckets stay as they were")
	assert.False(t, m.items[3].Checked)
}

func TestNClearsCurrentBucket(t *testing.T) {
	m := NewModel(sampleItems())
	m = applyKey(t, m, "n")

	assert.False(t, m.items[0].Checked)
	assert.False(t, m.items[1].Checked)
	assert.True(t, m.items[2].Checked)
}

func TestDownMovesCursorIntoNeedsReviewThenA(t *testing.T) {
	m := NewModel(sampleItems())
	for i := 0; i < 3; i++ {
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m = next.(Model)
	}
	assert.Equal(t, 3, m.cursor)
	m = applyKey(t, m, "a")
	assert.True(t, m.items[3].Checked)
	assert.True(t, m.items[0].Checked, "safe bucket unchanged")
}

func TestEnterSubmitsSelection(t *testing.T) {
	m := NewModel(sampleItems())
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	assert.True(t, m.Submitted())
	assert.False(t, m.Cancelled())
	assert.NotNil(t, cmd)

	got := names(m.Selected())
	assert.Equal(t, []string{"safe-a", "safe-b", "squash-a"}, got)
}

func TestEscCancels(t *testing.T) {
	m := NewModel(sampleItems())
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	assert.True(t, m.Cancelled())
	assert.False(t, m.Submitted())
}

func TestQCancels(t *testing.T) {
	m := NewModel(sampleItems())
	m = applyKey(t, m, "q")
	assert.True(t, m.Cancelled())
}

func TestViewShowsGroupsAndHelp(t *testing.T) {
	m := NewModel(sampleItems())
	view := m.View()
	assert.Contains(t, view, "Safe to delete")
	assert.Contains(t, view, "Squash-merged")
	assert.Contains(t, view, "Needs review")
	assert.Contains(t, view, "safe-a")
	assert.Contains(t, view, "[x]")
	assert.Contains(t, view, "[ ]")
	assert.Contains(t, view, "space toggle")
	assert.Contains(t, view, "q/esc cancel")
}

func TestRunNonTerminalReturnsPrechecked(t *testing.T) {
	selected, cancelled, err := Run(sampleItems(), strings.NewReader(""), nil)
	require.NoError(t, err)
	assert.False(t, cancelled)
	assert.Equal(t, []string{"safe-a", "safe-b", "squash-a"}, names(selected))
}

func names(items []Item) []string {
	out := make([]string, len(items))
	for i, item := range items {
		out[i] = item.Name
	}
	return out
}
