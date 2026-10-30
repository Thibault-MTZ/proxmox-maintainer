// Package tui contains the read-only beta interface. Commands return messages;
// Update changes state and View renders it. No maintenance runs from View.
package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Thibault-MTZ/proxmox-maintainer/internal/config"
	"github.com/Thibault-MTZ/proxmox-maintainer/internal/engine"
	"github.com/Thibault-MTZ/proxmox-maintainer/internal/jobs"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

var accent = lipgloss.NewStyle().Foreground(lipgloss.Color("#64FFDA")).Bold(true)
var muted = lipgloss.NewStyle().Foreground(lipgloss.Color("#8297AA"))
var frame = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#236276")).Padding(0, 1)

type loaded struct {
	targets []engine.Target
	err     error
}
type inspected struct {
	values []engine.Inspection
	errors []string
}
type planned struct {
	plans  []engine.Plan
	errors []string
}
type jobFinished struct {
	outcome jobs.Outcome
	err     error
	waiting bool
}

type Model struct {
	backend                   *engine.Engine
	cfg                       config.Config
	dir                       string
	demo, busy, advanced      bool
	targets                   []engine.Target
	selected                  map[string]bool
	plans                     []engine.Plan
	table                     table.Model
	spinner                   spinner.Model
	viewport                  viewport.Model
	filter, host, backup      textinput.Model
	confirm                   textinput.Model
	confirmAction, jobDir     string
	filtering                 bool
	field, tab, width, height int
	detail, planText, status  string
	logs                      []string
}

func New(e *engine.Engine, c config.Config, dir string) Model {
	t := table.New(table.WithColumns(columns(100)), table.WithFocused(true), table.WithHeight(12))
	style := table.DefaultStyles()
	style.Header = style.Header.Foreground(lipgloss.Color("#64FFDA")).Bold(true)
	style.Selected = style.Selected.Foreground(lipgloss.Color("#D5F8F1")).Background(lipgloss.Color("#185568"))
	t.SetStyles(style)
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = accent
	f := textinput.New()
	f.Placeholder = "nom / ID / état"
	f.CharLimit = 80
	host := textinput.New()
	host.SetValue(c.Host)
	host.Placeholder = "vide = hôte PVE local"
	host.CharLimit = 200
	backup := textinput.New()
	backup.SetValue(c.BackupStorage)
	backup.Placeholder = "stockage de sauvegarde (usage futur)"
	backup.CharLimit = 100
	confirm := textinput.New()
	confirm.CharLimit = 30
	return Model{backend: e, cfg: c, dir: dir, demo: e.Demo, selected: map[string]bool{}, table: t, spinner: s, viewport: viewport.New(96, 15), filter: f, host: host, backup: backup, confirm: confirm, width: 100, height: 30, busy: true, status: "Chargement de l’inventaire…", detail: "Entrée sur une cible pour voir ses applications et services.", planText: "Cocher des invités actifs, puis P pour simuler les mises à jour APT."}
}

func columns(width int) []table.Column {
	return []table.Column{{Title: "Choix", Width: 5}, {Title: "Type", Width: 5}, {Title: "ID", Width: 8}, {Title: "Nom", Width: max(12, width-47)}, {Title: "État", Width: 10}}
}
func safe(s string) string {
	return strings.Map(func(r rune) rune {
		if (r < 32 && r != '\n' && r != '\t') || r == 127 {
			return -1
		}
		return r
	}, ansi.Strip(s))
}
func (m Model) Init() tea.Cmd                   { return tea.Batch(m.spinner.Tick, m.load()) }
func (m Model) WithAdvanced(enabled bool) Model { m.advanced = enabled; return m }
func (m Model) load() tea.Cmd {
	return func() tea.Msg { t, e := m.backend.Inventory(); return loaded{t, e} }
}
func (m *Model) rows() {
	rows := []table.Row{}
	q := strings.ToLower(m.filter.Value())
	for _, t := range m.targets {
		if !strings.Contains(strings.ToLower(t.ID+" "+t.Name+" "+t.Status+" "+t.Kind), q) {
			continue
		}
		choice := "[ ]"
		if m.selected[t.ID] {
			choice = "[x]"
		}
		state := "○ ARRÊTÉ"
		if t.Status == "running" {
			state = "● ACTIF"
		}
		kind := map[string]string{"host": "PVE", "ct": "LXC", "vm": "QEMU"}[t.Kind]
		rows = append(rows, table.Row{choice, kind, t.ID, safe(t.Name), state})
	}
	m.table.SetRows(rows)
}
func (m Model) current() string {
	row := m.table.SelectedRow()
	if len(row) > 2 {
		return row[2]
	}
	return ""
}
func (m Model) chosen() []string {
	out := []string{}
	for _, t := range m.targets {
		if m.selected[t.ID] {
			out = append(out, t.ID)
		}
	}
	return out
}
func (m *Model) log(s string) {
	m.logs = append(m.logs, time.Now().Format("15:04:05")+"  "+safe(s))
	m.status = s
}
func (m *Model) content() {
	switch m.tab {
	case 1:
		m.viewport.SetContent(m.detail)
	case 2:
		m.viewport.SetContent(m.planText)
	case 4:
		m.viewport.SetContent(strings.Join(m.logs, "\n"))
	}
}
func (m Model) inspect(ids []string) tea.Cmd {
	return func() tea.Msg {
		msg := inspected{}
		for _, id := range ids {
			if !m.advanced {
				p, e := m.backend.Plan(id)
				if e != nil {
					msg.errors = append(msg.errors, id+": "+e.Error())
				} else {
					msg.values = append(msg.values, engine.Inspection{Target: id, OS: "APT — cache existant", Packages: p.Packages})
				}
				continue
			}
			v, e := m.backend.Inspect(id)
			if e != nil {
				msg.errors = append(msg.errors, id+": "+e.Error())
			} else {
				msg.values = append(msg.values, v)
			}
		}
		return msg
	}
}
func (m Model) prepare(ids []string) tea.Cmd {
	return func() tea.Msg {
		msg := planned{}
		for _, id := range ids {
			p, e := m.backend.Plan(id)
			if e != nil {
				msg.errors = append(msg.errors, id+": "+e.Error())
			} else {
				msg.plans = append(msg.plans, p)
			}
		}
		return msg
	}
}
func (m Model) pollJob() tea.Cmd {
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg {
		data, err := os.ReadFile(filepath.Join(m.jobDir, "result.json"))
		if os.IsNotExist(err) {
			return jobFinished{waiting: true}
		}
		if err != nil {
			return jobFinished{err: err}
		}
		var result jobs.Outcome
		if err = json.Unmarshal(data, &result); err != nil {
			return jobFinished{err: err}
		}
		return jobFinished{outcome: result, waiting: !result.Finished}
	})
}
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.table.SetColumns(columns(max(60, msg.Width-6)))
		m.table.SetHeight(max(3, msg.Height-13))
		m.viewport.Width = max(30, msg.Width-8)
		m.viewport.Height = max(3, msg.Height-13)
	case loaded:
		m.busy = false
		if msg.err != nil {
			m.log("Inventaire : " + msg.err.Error())
			break
		}
		old := map[string]string{}
		for _, t := range m.targets {
			old[t.ID] = t.Identity
		}
		m.targets = msg.targets
		kept := map[string]bool{}
		for _, t := range m.targets {
			if m.selected[t.ID] && old[t.ID] == t.Identity {
				kept[t.ID] = true
			}
		}
		m.selected = kept
		m.plans = nil
		m.rows()
		m.log("Inventaire actualisé ; anciens plans invalidés.")
	case inspected:
		m.busy = false
		var b strings.Builder
		for _, v := range msg.values {
			fmt.Fprintf(&b, "CIBLE %s  //  %s\n\nPAQUETS CANDIDATS\n", v.Target, safe(v.OS))
			for _, p := range v.Packages {
				fmt.Fprintf(&b, "  %s %s → %s\n", safe(p.Name), safe(p.Installed), safe(p.Candidate))
			}
			if !m.advanced {
				b.WriteString("\n")
				continue
			}
			b.WriteString("\nAPPLICATIONS\n")
			for _, a := range v.Apps {
				versions, _ := json.Marshal(a.Versions)
				fmt.Fprintf(&b, "  %s  %s %s\n", safe(a.Name), safe(string(versions)), safe(a.Image))
			}
			fmt.Fprintf(&b, "\nSERVICES\n%s\n\nÉCOUTES LOCALES\n%s\n\nFICHIERS DE DÉPÔTS\n%s\n\nOBSERVATIONS\n%s\n\n", safe(strings.Join(v.Services, "\n")), safe(strings.Join(v.Ports, "\n")), safe(strings.Join(v.Repositories, "\n")), safe(strings.Join(v.Notes, "\n")))
		}
		b.WriteString(strings.Join(msg.errors, "\n"))
		m.detail = b.String()
		m.tab = 1
		m.content()
		m.viewport.GotoTop()
		m.log("Analyse terminée.")
	case planned:
		m.busy = false
		m.plans = msg.plans
		var b strings.Builder
		b.WriteString("SIMULATION // cache APT existant, sans actualisation\n\n")
		for _, p := range msg.plans {
			fmt.Fprintf(&b, "CIBLE %s  //  %d paquet(s)\n", p.Target, len(p.Packages))
			for _, pkg := range p.Packages {
				fmt.Fprintf(&b, "  %s  %s → %s\n", safe(pkg.Name), safe(pkg.Installed), safe(pkg.Candidate))
			}
			b.WriteString("\n")
		}
		b.WriteString(strings.Join(msg.errors, "\n"))
		b.WriteString("\n\nM : appliquer après confirmation et sauvegarde obligatoire.\nLes services peuvent redémarrer. Aucun reboot automatique.")
		m.planText = b.String()
		m.tab = 2
		m.content()
		m.viewport.GotoTop()
		m.log("Plan préparé, aucune modification.")
	case jobFinished:
		if msg.waiting {
			return m, m.pollJob()
		}
		if msg.err != nil {
			m.log("Lecture résultat : " + msg.err.Error())
		} else {
			data, _ := json.Marshal(msg.outcome)
			m.log(string(data))
		}
		m.jobDir = ""
		m.content()
	case spinner.TickMsg:
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case tea.KeyMsg:
		key := msg.String()
		if key == "ctrl+c" {
			return m, tea.Quit
		}
		if m.confirmAction != "" {
			if key == "esc" {
				m.confirmAction = ""
				m.confirm.Blur()
				return m, nil
			}
			if key == "enter" {
				phrase := "METTRE-A-JOUR"
				if m.confirmAction == "refresh" {
					phrase = "ACTUALISER"
				}
				if m.confirm.Value() != phrase {
					m.log("Confirmation incorrecte.")
					return m, nil
				}
				action := m.confirmAction
				m.confirmAction = ""
				m.confirm.Blur()
				if m.demo {
					m.log("DÉMO : action simulée, aucun paquet ni sauvegarde modifié.")
					return m, nil
				}
				request := jobs.Request{Action: action, Config: m.cfg, Plans: m.plans, IDs: m.chosen(), StateDir: m.dir}
				dir, err := jobs.Start(request)
				if err != nil {
					m.log(err.Error())
					return m, nil
				}
				m.jobDir = dir
				m.plans = nil
				m.tab = 4
				m.log("Tâche indépendante : " + dir + "/worker.log")
				m.content()
				return m, m.pollJob()
			}
			m.confirm, cmd = m.confirm.Update(msg)
			return m, cmd
		}
		if m.filtering {
			if key == "esc" || key == "enter" {
				m.filtering = false
				m.filter.Blur()
				return m, nil
			}
			m.filter, cmd = m.filter.Update(msg)
			m.rows()
			return m, cmd
		}
		if m.tab == 3 {
			switch key {
			case "esc":
				m.host.Blur()
				m.backup.Blur()
				m.tab = 0
			case "tab", "enter":
				m.field = 1 - m.field
				if m.field == 0 {
					m.backup.Blur()
					cmd = m.host.Focus()
				} else {
					m.host.Blur()
					cmd = m.backup.Focus()
				}
			case "ctrl+s":
				if m.busy {
					m.log("Attendre la fin de l’opération.")
					break
				}
				c := config.Config{Host: strings.TrimSpace(m.host.Value()), BackupStorage: strings.TrimSpace(m.backup.Value()), VMSSH: m.cfg.VMSSH}
				if strings.HasPrefix(c.Host, "-") || strings.ContainsAny(c.Host, " \n\r\t") {
					m.log("Alias SSH invalide.")
					break
				}
				if err := config.Save(m.dir, c); err != nil {
					m.log(err.Error())
				} else {
					m.cfg = c
					m.backend = engine.New(m.demo, c.Host, m.dir)
					m.backend.VMSSH = c.VMSSH
					m.selected = map[string]bool{}
					m.tab = 0
					m.busy = true
					return m, m.load()
				}
			default:
				if m.field == 0 {
					m.host, cmd = m.host.Update(msg)
				} else {
					m.backup, cmd = m.backup.Update(msg)
				}
			}
			return m, cmd
		}
		switch key {
		case "q":
			return m, tea.Quit
		case "1", "2", "3", "5":
			m.tab = map[string]int{"1": 0, "2": 1, "3": 2, "5": 4}[key]
			m.content()
		case "c", "4":
			m.tab = 3
			m.field = 0
			return m, m.host.Focus()
		case "r":
			if !m.busy {
				m.busy = true
				m.log("Lecture de l’inventaire…")
				return m, m.load()
			}
		case "/":
			if m.tab == 0 {
				m.filtering = true
				return m, m.filter.Focus()
			}
		case " ":
			if m.tab == 0 && !m.busy {
				id := m.current()
				if id != "" {
					m.selected[id] = !m.selected[id]
					m.rows()
				}
			}
		case "x":
			m.selected = map[string]bool{}
			m.rows()
		case "enter":
			if m.tab == 0 && !m.busy && m.current() != "" {
				m.busy = true
				return m, m.inspect([]string{m.current()})
			}
		case "a", "p":
			if m.busy {
				return m, nil
			}
			ids := m.chosen()
			if len(ids) == 0 {
				m.log("Cocher au moins une cible avec Espace.")
				break
			}
			m.busy = true
			if key == "p" {
				m.log("Simulation APT…")
				return m, m.prepare(ids)
			}
			m.log("Analyse des cibles…")
			return m, m.inspect(ids)
		case "m", "u":
			if m.busy || m.jobDir != "" {
				m.log("Une opération est déjà en cours.")
				break
			}
			if key == "m" {
				if len(m.plans) == 0 {
					m.log("Préparer un plan avec P avant application.")
					break
				}
				if m.cfg.BackupStorage == "" && !m.demo {
					m.log("Configurer le stockage de sauvegarde avec C.")
					break
				}
				for _, p := range m.plans {
					if p.Target == "host" {
						m.log("Retirer l’hôte du plan ; maintenance PVE non prise en charge.")
						return m, nil
					}
				}
				m.confirmAction = "apply"
			} else {
				if len(m.chosen()) == 0 {
					m.log("Sélection vide.")
					break
				}
				m.confirmAction = "refresh"
			}
			m.confirm.SetValue("")
			return m, m.confirm.Focus()
		case "e":
			if m.tab == 2 {
				if err := config.SaveJSON(filepath.Join(m.dir, "plan.json"), m.plans); err != nil {
					m.log(err.Error())
				} else {
					m.log("Plan exporté : " + filepath.Join(m.dir, "plan.json"))
				}
			}
		default:
			if m.tab == 0 {
				m.table, cmd = m.table.Update(msg)
			} else {
				m.viewport, cmd = m.viewport.Update(msg)
			}
		}
	}
	return m, cmd
}

func (m Model) View() string {
	if m.width < 60 || m.height < 18 {
		return "Proxmox Maintainer // BÊTA\nAgrandir le terminal (60×18 minimum). Q : quitter."
	}
	mode := "LIVE / ACTIONS CONFIRMÉES"
	if m.demo {
		mode = "DÉMO / DONNÉES FICTIVES"
	}
	edition := "TUI MINIMAL"
	if m.advanced {
		edition = "TUI AVANCÉ / EXPÉRIMENTAL"
	}
	header := accent.Render("◈ PROXMOX MAINTAINER") + muted.Render("   BÊTA  //  "+mode) + "\n" + muted.Render(edition)
	active, selected := 0, 0
	for _, t := range m.targets {
		if t.Status == "running" {
			active++
		}
		if m.selected[t.ID] {
			selected++
		}
	}
	activity := "● PRÊT"
	if m.busy {
		activity = m.spinner.View() + " LECTURE"
	}
	metrics := fmt.Sprintf("CIBLES %02d   ACTIVES %02d   CHOIX %02d   %s", len(m.targets), active, selected, activity)
	names := []string{"1 Inventaire", "2 Détails", "3 Plan", "4 Config", "5 Journal"}
	tabs := []string{}
	for i, n := range names {
		if i == m.tab {
			tabs = append(tabs, accent.Render("["+n+"]"))
		} else {
			tabs = append(tabs, muted.Render(n))
		}
	}
	var body string
	switch m.tab {
	case 0:
		body = m.table.View()
	case 3:
		body = "CONNEXION SSH (vide = PVE local)\n" + m.host.View() + "\n\nSTOCKAGE DE SAUVEGARDE\n" + m.backup.View() + "\n\nTab : champ suivant · Ctrl+S : enregistrer · Échap : retour\nVM : alias SSH à définir via --vm-ssh ou config.json.\nEnregistrer ne lance aucune maintenance."
	default:
		body = m.viewport.View()
	}
	if m.filtering {
		body = m.filter.View() + "\n" + body
	}
	if m.confirmAction != "" {
		phrase := "METTRE-A-JOUR"
		details := "Sauvegarde : " + m.cfg.BackupStorage + "\nRedémarrages de services possibles. Backups conservés.\nVolumes externes et bases déportées pas forcément couverts.\nVM : snapshot potentiellement crash-consistent, restauration non testée."
		if m.confirmAction == "refresh" {
			phrase = "ACTUALISER"
			details = "Actualise les index APT des cibles cochées. Aucun paquet installé."
		}
		body = "CONFIRMATION // " + m.confirmAction + "\n\n" + details + "\n\nTaper " + phrase + " puis Entrée (Échap : annuler)\n" + m.confirm.View()
	}
	body = frame.Width(max(30, m.width-4)).Height(max(5, m.height-10)).Render(body)
	help := "↑↓ naviguer · Espace cocher · Entrée détails · / filtre · P plan\nA analyser · U index · M appliquer · E export · C config · Q quitter"
	return header + "\n" + accent.Render(metrics) + "\n\n" + strings.Join(tabs, "  ") + "\n" + body + "\n" + muted.Render(safe(m.status)) + "\n" + muted.Render(help)
}
