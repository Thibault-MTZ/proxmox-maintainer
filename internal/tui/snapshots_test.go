package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"os"
	"path/filepath"
	"testing"
)

// Optional developer snapshots: data always comes from the disconnected demo.
func TestDemoSnapshots(t *testing.T) {
	dir := os.Getenv("MAINT_SCREENSHOT_DIR")
	if dir == "" {
		t.Skip("optional demo render")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	lipgloss.SetColorProfile(termenv.TrueColor)
	m := demoModel(t)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 112, Height: 30})
	m = next.(Model)
	if err := os.WriteFile(filepath.Join(dir, "inventory.ansi"), []byte(m.View()), 0644); err != nil {
		t.Fatal(err)
	}
	next, _ = m.Update(m.prepare([]string{"101", "qVM201"})())
	m = next.(Model)
	if err := os.WriteFile(filepath.Join(dir, "updates.ansi"), []byte(m.View()), 0644); err != nil {
		t.Fatal(err)
	}
}
