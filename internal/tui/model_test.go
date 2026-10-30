package tui

import (
	"github.com/Thibault-MTZ/proxmox-maintainer/internal/config"
	"github.com/Thibault-MTZ/proxmox-maintainer/internal/engine"
	tea "github.com/charmbracelet/bubbletea"
	"strings"
	"testing"
)

func demoModel(t *testing.T) Model {
	t.Helper()
	m := New(engine.New(true, "", t.TempDir()), config.Config{}, t.TempDir())
	msg := m.load()()
	next, _ := m.Update(msg)
	return next.(Model)
}
func key(m Model, r rune) (Model, tea.Cmd) {
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	return next.(Model), cmd
}
func TestDemoHasNoUserDataAndSelection(t *testing.T) {
	m := demoModel(t)
	if len(m.targets) < 3 {
		t.Fatal("inventory")
	}
	view := m.View()
	if !strings.Contains(view, "DÉMO") || strings.Contains(view, "192.168") {
		t.Fatal("demo data")
	}
	m, _ = key(m, ' ')
	if !m.selected["host"] {
		t.Fatal("selection")
	}
	m, _ = key(m, 'x')
	if len(m.chosen()) != 0 {
		t.Fatal("clear")
	}
}
func TestPlanBeforeApplyAndDemoConfirmation(t *testing.T) {
	m := demoModel(t)
	m, _ = key(m, 'm')
	if m.confirmAction != "" {
		t.Fatal("empty plan accepted")
	}
	m.selected["101"] = true
	m, cmd := key(m, 'p')
	if cmd == nil {
		t.Fatal("missing command")
	}
	next, _ := m.Update(cmd())
	m = next.(Model)
	if len(m.plans) != 1 {
		t.Fatal("plan")
	}
	m, _ = key(m, 'm')
	if m.confirmAction != "apply" {
		t.Fatal("confirmation")
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if m.confirmAction == "" {
		t.Fatal("empty phrase accepted")
	}
	m.confirm.SetValue("METTRE-A-JOUR")
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if m.jobDir != "" || m.confirmAction != "" || !strings.Contains(m.status, "DÉMO") {
		t.Fatal("demo created worker")
	}
}
func TestReloadInvalidatesPlanAndMissingTargets(t *testing.T) {
	m := demoModel(t)
	m.selected["101"] = true
	m.plans = []engine.Plan{{Target: "101"}}
	next, _ := m.Update(loaded{targets: []engine.Target{{ID: "host", Identity: "new"}}})
	m = next.(Model)
	if len(m.plans) != 0 || len(m.selected) != 0 {
		t.Fatal("stale state")
	}
}
func TestMinimalInspectionAndAdvancedMode(t *testing.T) {
	m := demoModel(t)
	msg := m.inspect([]string{"101"})()
	next, _ := m.Update(msg)
	m = next.(Model)
	if strings.Contains(m.detail, "SERVICES") {
		t.Fatal("minimal does full inspection")
	}
	m = m.WithAdvanced(true)
	next, _ = m.Update(m.inspect([]string{"101"})())
	m = next.(Model)
	if !strings.Contains(m.detail, "APPLICATIONS") {
		t.Fatal("advanced missing")
	}
}
func TestSmallTerminal(t *testing.T) {
	m := demoModel(t)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 40, Height: 10})
	m = next.(Model)
	if !strings.Contains(m.View(), "Agrandir") {
		t.Fatal("small terminal")
	}
}
func TestEscCancelsConfirmationAndSanitizesOutput(t *testing.T) {
	m := demoModel(t)
	m.confirmAction = "apply"
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	if m.confirmAction != "" {
		t.Fatal("cancel")
	}
	if safe("\x1b[31mtest\x00") != "test" {
		t.Fatal("escape sequence")
	}
}
