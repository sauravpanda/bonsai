package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestPickerGuidesSelectionAndRequiresReview(t *testing.T) {
	m := NewPicker("clean", []Item{{ID: "one", Label: "one"}})

	m = updatePicker(t, m, "enter")
	if m.done || m.reviewing || !strings.Contains(m.notice, "press Space") {
		t.Fatalf("empty Enter should explain selection, got %+v", m)
	}

	m = updatePicker(t, m, " ")
	if !m.items[0].Selected {
		t.Fatal("Space did not select the focused row")
	}
	m = updatePicker(t, m, "enter")
	if m.done || !m.reviewing {
		t.Fatalf("Enter should open review without confirming, got %+v", m)
	}
	m = updatePicker(t, m, "y")
	if !m.done || m.quit {
		t.Fatalf("review confirmation did not complete, got %+v", m)
	}
}

func TestPickerProtectsRowsUnlessForceIsEnabled(t *testing.T) {
	items := []Item{
		{ID: "safe", Label: "safe"},
		{ID: "protected", Label: "protected", Protected: true},
	}
	m := NewPicker("clean", items)
	m.cursor = 1
	m = updatePicker(t, m, " ")
	if m.items[1].Selected || !strings.Contains(m.notice, "--force") {
		t.Fatalf("protected row should remain view-only, got %+v", m)
	}

	forced := NewPickerWithOptions("clean", items, PickerOptions{AllowProtected: true})
	forced.cursor = 1
	forced = updatePicker(t, forced, " ")
	if !forced.items[1].Selected {
		t.Fatal("--force picker did not allow explicit protected-row selection")
	}
}

func TestPickerBulkSelectionSkipsProtectedRows(t *testing.T) {
	m := NewPickerWithOptions("clean", []Item{
		{ID: "safe", Label: "safe"},
		{ID: "protected", Label: "protected", Protected: true},
	}, PickerOptions{AllowProtected: true})

	m = updatePicker(t, m, "a")
	if !m.items[0].Selected || m.items[1].Selected {
		t.Fatalf("bulk selection should be safe-only, got %+v", m.items)
	}
	if !strings.Contains(m.notice, "skipped 1 protected") {
		t.Fatalf("bulk selection should explain skipped rows, got %q", m.notice)
	}
}

func TestPickerKeepsCursorInVisibleWindow(t *testing.T) {
	items := make([]Item, 30)
	for i := range items {
		items[i] = Item{ID: string(rune('a' + i)), Label: "item"}
	}
	m := NewPicker("clean", items)
	m.height = 16
	m.cursor = 20
	start, end := m.visibleRange()
	if start > m.cursor || end <= m.cursor {
		t.Fatalf("cursor %d outside visible range [%d,%d)", m.cursor, start, end)
	}
	if end-start >= len(items) {
		t.Fatalf("expected a bounded viewport, got [%d,%d)", start, end)
	}
}

func updatePicker(t *testing.T, model Model, key string) Model {
	t.Helper()
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
	return updated.(Model)
}
