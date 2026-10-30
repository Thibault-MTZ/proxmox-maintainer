// Package engine discovers Proxmox guests and guards explicitly configured APT maintenance.
package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Target struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	Name     string `json:"name"`
	Status   string `json:"status"`
	Identity string `json:"identity"`
}
type Package struct {
	Name      string `json:"name"`
	Installed string `json:"installed"`
	Candidate string `json:"candidate"`
}
type Plan struct {
	Target      string    `json:"target"`
	Identity    string    `json:"identity"`
	Packages    []Package `json:"packages"`
	Fingerprint string    `json:"fingerprint"`
	Date        string    `json:"date"`
	Demo        bool      `json:"demo"`
}
type App struct {
	ID       string            `json:"id"`
	Name     string            `json:"name"`
	Status   string            `json:"status"`
	Update   string            `json:"update"`
	Image    string            `json:"image,omitempty"`
	Versions map[string]string `json:"versions,omitempty"`
}
type Inspection struct {
	Target       string    `json:"target"`
	Identity     string    `json:"identity"`
	OS           string    `json:"os"`
	Packages     []Package `json:"packages"`
	Apps         []App     `json:"apps"`
	Services     []string  `json:"services"`
	Ports        []string  `json:"ports"`
	Repositories []string  `json:"repositories"`
	Notes        []string  `json:"notes"`
}
type VersionCheck struct {
	Name     string `json:"name"`
	Expected string `json:"expected"`
	Actual   string `json:"actual"`
	Matches  bool   `json:"matches"`
}
type Verification struct {
	Versions       []VersionCheck `json:"versions"`
	Remaining      []Package      `json:"remaining"`
	FailedServices []string       `json:"failed_services"`
	Notes          []string       `json:"notes"`
}
type Result struct {
	Target       string       `json:"target"`
	Status       string       `json:"status"`
	Demo         bool         `json:"demo"`
	Notes        []string     `json:"notes"`
	Verification Verification `json:"verification"`
}
type Engine struct {
	Demo     bool
	Host     string
	StateDir string
	// VMSSH maps qVM-prefixed IDs to existing root SSH aliases reachable from PVE.
	VMSSH map[string]string
	Exec  func(args []string, mutation bool) (string, error)
}

var idRE = regexp.MustCompile(`^(host|[1-9][0-9]*|qVM[1-9][0-9]*)$`)
var hostRE = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.:\-]*$`)
var storageRE = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.-]*$`)
var machineRE = regexp.MustCompile(`^[a-f0-9]{32}$`)
var instRE = regexp.MustCompile(`^Inst ([a-z0-9][a-z0-9+.:-]*)(?: \[([^\]\s]+)\])? \(([^\s()]+)(?: [^\r\n]*)?\)$`)

func New(demo bool, host, stateDir string) *Engine {
	if stateDir == "" {
		home, _ := os.UserHomeDir()
		stateDir = filepath.Join(home, ".local", "state", "homelab-maintainer")
	}
	e := &Engine{Demo: demo, Host: host, StateDir: stateDir, VMSSH: map[string]string{}}
	e.Exec = e.execute
	return e
}
func hash(v any) string {
	raw, _ := json.Marshal(v)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }
func (e *Engine) execute(args []string, mutation bool) (string, error) {
	if len(args) == 0 {
		return "", errors.New("commande vide")
	}
	argv := append([]string{"env", "LC_ALL=C"}, args...)
	if e.Host != "" {
		if !hostRE.MatchString(e.Host) {
			return "", errors.New("hôte SSH invalide")
		}
		quoted := make([]string, len(argv))
		for i, a := range argv {
			quoted[i] = shellQuote(a)
		}
		argv = []string{"ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=10", "root@" + e.Host, strings.Join(quoted, " ")}
	}
	ctx := context.Background()
	cancel := func() {}
	if !mutation {
		ctx, cancel = context.WithTimeout(ctx, 60*time.Second)
	}
	defer cancel()
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	// Never relay stderr, which can contain repository credentials.
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("commande %s indisponible ou en échec", args[0])
	}
	return string(out), nil
}
func (e *Engine) run(args []string, mutation bool) (string, error) {
	if e.Demo {
		return "", errors.New("commande interdite en démo")
	}
	if e.Exec == nil {
		return "", errors.New("exécuteur absent")
	}
	out, err := e.Exec(args, mutation)
	if err != nil {
		return "", fmt.Errorf("échec de la commande %s", args[0])
	}
	return out, nil
}
func (e *Engine) guest(id string, args []string, mutation bool) (string, error) {
	if !idRE.MatchString(id) {
		return "", errors.New("cible invalide")
	}
	if strings.HasPrefix(id, "qVM") {
		alias, err := e.vmAlias(id)
		if err != nil {
			return "", err
		}
		quoted := []string{shellQuote("env"), shellQuote("LC_ALL=C")}
		for _, arg := range args {
			quoted = append(quoted, shellQuote(arg))
		}
		// This SSH process runs on PVE; e.run may itself connect to PVE by SSH.
		return e.run([]string{"ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=10", "root@" + alias, strings.Join(quoted, " ")}, mutation)
	}
	if id != "host" {
		args = append([]string{"pct", "exec", id, "--", "env", "LC_ALL=C"}, args...)
	}
	return e.run(args, mutation)
}
func (e *Engine) vmAlias(id string) (string, error) {
	alias := e.VMSSH[id]
	if alias == "" {
		return "", errors.New("VM sans alias SSH explicite ; configurer vm_ssh pour cette cible")
	}
	if !hostRE.MatchString(alias) {
		return "", errors.New("alias SSH VM invalide")
	}
	return alias, nil
}
func lines(s string) []string {
	result := []string{}
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) != "" {
			result = append(result, strings.TrimSpace(line))
		}
	}
	return result
}
func (e *Engine) Inventory() ([]Target, error) {
	if e.Demo {
		return demoTargets(), nil
	}
	name, err := e.run([]string{"hostname"}, false)
	if err != nil {
		return nil, err
	}
	targets := []Target{{"host", "host", strings.TrimSpace(name), "running", hash([]string{e.Host, name})}}
	for _, kind := range []string{"ct", "vm"} {
		tool := "pct"
		if kind == "vm" {
			tool = "qm"
		}
		out, err := e.run([]string{tool, "list"}, false)
		if err != nil {
			return nil, err
		}
		for _, row := range lines(out) {
			p := strings.Fields(row)
			if len(p) < 3 {
				continue
			}
			if _, err := strconv.Atoi(p[0]); err != nil {
				continue
			}
			id, status, name := p[0], p[1], p[len(p)-1]
			if kind == "vm" {
				id = "qVM" + id
				name = p[1]
				status = p[2]
			}
			targets = append(targets, Target{id, kind, name, status, hash([]string{e.Host, kind, id, name})})
		}
	}
	return targets, nil
}
func (e *Engine) find(id string, active, maintenance bool) (Target, error) {
	if !idRE.MatchString(id) {
		return Target{}, errors.New("identifiant invalide")
	}
	targets, err := e.Inventory()
	if err != nil {
		return Target{}, err
	}
	for _, t := range targets {
		if t.ID != id {
			continue
		}
		if maintenance && t.Kind == "host" {
			return Target{}, errors.New("opération interdite sur l’hôte Proxmox")
		}
		if active && t.Status != "running" {
			return Target{}, errors.New("cible arrêtée : aucun démarrage automatique")
		}
		if active && t.Kind == "vm" && !e.Demo {
			if _, err := e.vmAlias(id); err != nil {
				return Target{}, err
			}
		}
		if active && t.Kind != "host" && !e.Demo {
			tool, numericID := "pct", id
			if t.Kind == "vm" {
				tool = "qm"
				numericID = strings.TrimPrefix(id, "qVM")
			}
			config, err := e.run([]string{tool, "config", numericID}, false)
			if err != nil {
				return Target{}, err
			}
			for _, line := range lines(config) {
				if strings.HasPrefix(line, "lock:") || strings.HasPrefix(line, "template: 1") {
					return Target{}, errors.New("invité verrouillé ou modèle : opération refusée")
				}
			}
		}
		return t, nil
	}
	return Target{}, errors.New("cible absente de l’inventaire")
}
func (e *Engine) identity(t Target) (string, error) {
	if e.Demo {
		return t.Identity, nil
	}
	machine, err := e.guest(t.ID, []string{"cat", "/etc/machine-id"}, false)
	if err != nil {
		return "", err
	}
	machine = strings.TrimSpace(machine)
	if !machineRE.MatchString(machine) {
		return "", errors.New("identité machine non vérifiable")
	}
	config := []string{}
	if t.Kind == "ct" {
		raw, err := e.run([]string{"pct", "config", t.ID}, false)
		if err != nil {
			return "", err
		}
		for _, line := range lines(raw) {
			for _, key := range []string{"hostname:", "rootfs:", "ostype:", "unprivileged:"} {
				if strings.HasPrefix(line, key) {
					config = append(config, line)
					break
				}
			}
		}
		sort.Strings(config)
	}
	alias, pveMachine := "", ""
	if t.Kind == "vm" {
		alias, err = e.vmAlias(t.ID)
		if err != nil {
			return "", err
		}
		pveMachine, err = e.run([]string{"cat", "/etc/machine-id"}, false)
		if err != nil {
			return "", err
		}
		pveMachine = strings.TrimSpace(pveMachine)
		if !machineRE.MatchString(pveMachine) {
			return "", errors.New("identité PVE non vérifiable")
		}
		raw, err := e.run([]string{"qm", "config", strings.TrimPrefix(t.ID, "qVM")}, false)
		if err != nil {
			return "", err
		}
		diskRE := regexp.MustCompile(`^(scsi|virtio|sata|ide)[0-9]+:`)
		for _, line := range lines(raw) {
			if diskRE.MatchString(line) || strings.HasPrefix(line, "smbios1:") || strings.HasPrefix(line, "boot:") || strings.HasPrefix(line, "efidisk0:") || strings.HasPrefix(line, "tpmstate0:") {
				config = append(config, line)
			}
		}
		if len(config) == 0 {
			return "", errors.New("identité de configuration VM non vérifiable")
		}
		sort.Strings(config)
	}
	return hash([]string{e.Host, t.ID, machine, strings.Join(config, "\n"), alias, pveMachine}), nil
}

// ParseSimulation accepts only recognized APT Inst records and never removals.
func ParseSimulation(out string) ([]Package, error) {
	packages := []Package{}
	seen := map[string]bool{}
	for _, line := range lines(out) {
		if strings.HasPrefix(line, "Remv ") || strings.HasPrefix(line, "Purg ") {
			return nil, errors.New("suppression APT refusée")
		}
		if !strings.HasPrefix(line, "Inst ") {
			continue
		}
		m := instRE.FindStringSubmatch(line)
		if m == nil || strings.ContainsAny(m[3], ";`$\\") {
			return nil, errors.New("simulation APT ambiguë")
		}
		if seen[m[1]] {
			return nil, errors.New("paquet dupliqué")
		}
		seen[m[1]] = true
		old := m[2]
		if old == "" {
			old = "(nouveau)"
		}
		packages = append(packages, Package{m[1], old, m[3]})
	}
	sort.Slice(packages, func(i, j int) bool { return packages[i].Name < packages[j].Name })
	return packages, nil
}
func (e *Engine) packages(id string) ([]Package, error) {
	out, err := e.guest(id, []string{"apt-get", "-s", "--no-remove", "upgrade"}, false)
	if err != nil {
		return nil, err
	}
	return ParseSimulation(out)
}
func (e *Engine) installed(id string) (map[string]string, error) {
	out, err := e.guest(id, []string{"dpkg-query", "-W", "-f=${binary:Package}\t${Version}\t${db:Status-Abbrev}\n"}, false)
	if err != nil {
		return nil, err
	}
	result := map[string]string{}
	for _, row := range strings.Split(out, "\n") {
		p := strings.Split(row, "\t")
		if len(p) == 3 && strings.HasPrefix(p[2], "ii") {
			result[p[0]] = p[1]
		}
	}
	return result, nil
}
func (e *Engine) Inspect(id string) (Inspection, error) {
	t, err := e.find(id, false, false)
	if err != nil {
		return Inspection{}, err
	}
	r := Inspection{Target: id, Identity: t.Identity, OS: "inconnu", Packages: []Package{}, Apps: []App{}, Services: []string{}, Ports: []string{}, Repositories: []string{}, Notes: []string{}}
	if t.Status != "running" {
		r.Notes = append(r.Notes, "Cible arrêtée : inspection invitée non exécutée.")
		return r, nil
	}
	if t.Kind == "vm" && !e.Demo {
		if _, err = e.vmAlias(id); err != nil {
			r.Notes = append(r.Notes, err.Error())
			return r, nil
		}
	}
	if e.Demo {
		r.OS = "Debian GNU/Linux 13 (démo)"
		r.Packages = demoPackages()
		r.Apps = []App{{ID: "caddy", Name: "Caddy", Status: "géré par APT", Update: "via les paquets du plan APT", Versions: map[string]string{"caddy": "2.10.2"}}, {ID: "docker:jellyfin", Name: "Jellyfin", Image: "jellyfin/jellyfin:stable", Status: "conteneur actif", Update: "manuel"}}
		r.Services = []string{"ssh.service", "caddy.service"}
		r.Ports = []string{"tcp LISTEN 0 128 *:22 *:*"}
		r.Repositories = []string{"/etc/apt/sources.list.d/debian.sources"}
		r.Notes = []string{"Simulation locale ; aucun accès réseau ni mutation serveur."}
		return r, nil
	}
	// Refuse locked/template guests before entering them, including read-only discovery.
	if _, err = e.find(id, true, false); err != nil {
		return r, err
	}
	r.Identity, err = e.identity(t)
	if err != nil {
		return r, err
	}
	raw, err := e.guest(id, []string{"cat", "/etc/os-release"}, false)
	if err != nil {
		return r, err
	}
	for _, line := range lines(raw) {
		if strings.HasPrefix(line, "PRETTY_NAME=") {
			r.OS = strings.Trim(strings.TrimPrefix(line, "PRETTY_NAME="), "\"")
		}
	}
	r.Packages, err = e.packages(id)
	if err != nil {
		r.Packages = []Package{}
		r.Notes = append(r.Notes, "Simulation APT indisponible.")
	}
	scans := []struct {
		key  string
		args []string
	}{{"services", []string{"systemctl", "list-units", "--type=service", "--state=running", "--no-legend", "--no-pager", "--plain"}}, {"ports", []string{"ss", "-H", "-lntu"}}, {"repositories", []string{"find", "/etc/apt", "-maxdepth", "2", "-type", "f", "(", "-name", "*.list", "-o", "-name", "*.sources", ")"}}}
	for _, scan := range scans {
		out, err := e.guest(id, scan.args, false)
		if err != nil {
			r.Notes = append(r.Notes, scan.key+" : indisponible.")
			continue
		}
		rows := lines(out)
		switch scan.key {
		case "services":
			for _, line := range rows {
				r.Services = append(r.Services, strings.Fields(line)[0])
			}
		case "ports":
			r.Ports = rows
		case "repositories":
			r.Repositories = rows
		}
	}
	if installed, err := e.installed(id); err == nil {
		for _, c := range catalogue {
			versions := map[string]string{}
			for name, version := range installed {
				for _, pkg := range c.packages {
					if strings.Split(name, ":")[0] == pkg {
						versions[name] = version
					}
				}
			}
			if len(versions) > 0 {
				r.Apps = append(r.Apps, App{ID: c.id, Name: c.name, Status: "géré par APT", Update: "via les paquets du plan APT", Versions: versions})
			}
		}
	}
	if out, err := e.guest(id, []string{"docker", "ps", "--format", "{{.Names}}\t{{.Image}}"}, false); err == nil {
		for _, line := range lines(out) {
			p := strings.SplitN(line, "\t", 2)
			if len(p) == 2 {
				image := p[1]
				if at := strings.Index(image, "@"); at >= 0 && strings.Contains(image[:at], ":") && !strings.Contains(image[:at], "/") {
					image = "[redacted]@" + image[at+1:]
				}
				r.Apps = append(r.Apps, App{ID: "docker:" + p[0], Name: p[0], Image: image, Status: "conteneur actif", Update: "manuel"})
			}
		}
	}
	r.Notes = append(r.Notes, "Versions candidates issues du cache APT ; actualisation explicite disponible.")
	if t.Kind == "vm" {
		r.Notes = append(r.Notes, "VM via SSH depuis PVE ; sans QGA, sauvegarde snapshot potentiellement crash-consistent.")
	}
	return r, nil
}
func fingerprint(p Plan) string {
	return hash(struct {
		Target   string
		Identity string
		Packages []Package
	}{p.Target, p.Identity, p.Packages})
}
func (e *Engine) Plan(id string) (Plan, error) {
	t, err := e.find(id, true, false)
	if err != nil {
		return Plan{}, err
	}
	identity, err := e.identity(t)
	if err != nil {
		return Plan{}, err
	}
	packages := demoPackages()
	if !e.Demo {
		packages, err = e.packages(id)
		if err != nil {
			return Plan{}, err
		}
	}
	p := Plan{Target: id, Identity: identity, Packages: packages, Date: time.Now().UTC().Format(time.RFC3339), Demo: e.Demo}
	p.Fingerprint = fingerprint(p)
	return p, nil
}
func (e *Engine) Refresh(id string) error {
	if _, err := e.find(id, true, true); err != nil {
		return err
	}
	if e.Demo {
		return nil
	}
	_, err := e.guest(id, []string{"apt-get", "update"}, true)
	return err
}
func (e *Engine) health(id string) error {
	out, err := e.guest(id, []string{"dpkg", "--audit"}, false)
	if err != nil {
		return err
	}
	if strings.TrimSpace(out) != "" {
		return errors.New("dpkg signale des paquets incomplets")
	}
	_, err = e.guest(id, []string{"apt-get", "check"}, false)
	return err
}
func (e *Engine) preflight(id string) error {
	out, err := e.guest(id, []string{"df", "-Pk", "/"}, false)
	if err != nil {
		return err
	}
	rows := lines(out)
	if len(rows) == 0 {
		return errors.New("espace libre non vérifiable")
	}
	p := strings.Fields(rows[len(rows)-1])
	if len(p) < 6 {
		return errors.New("espace libre non vérifiable")
	}
	available, err := strconv.ParseInt(p[3], 10, 64)
	if err != nil || available < 512*1024 {
		return errors.New("au moins 512 Mio libres requis dans l’invité")
	}
	return e.health(id)
}
func (e *Engine) verify(id string, packages []Package) Verification {
	v := Verification{Versions: []VersionCheck{}, Remaining: []Package{}, FailedServices: []string{}, Notes: []string{}}
	if installed, err := e.installed(id); err != nil {
		v.Notes = append(v.Notes, "Versions installées non vérifiables.")
	} else {
		for _, p := range packages {
			actual, ok := installed[p.Name]
			if !ok && !strings.Contains(p.Name, ":") {
				matches := []string{}
				for name, version := range installed {
					if strings.Split(name, ":")[0] == p.Name {
						matches = append(matches, version)
					}
				}
				if len(matches) == 1 {
					actual = matches[0]
				}
			}
			matched := actual == p.Candidate
			v.Versions = append(v.Versions, VersionCheck{p.Name, p.Candidate, actual, matched})
			if !matched {
				v.Notes = append(v.Notes, "Version installée différente du plan : "+p.Name)
			}
		}
	}
	if e.health(id) != nil {
		v.Notes = append(v.Notes, "Contrôle dpkg/APT incomplet ou en échec.")
	}
	if remaining, err := e.packages(id); err != nil {
		v.Notes = append(v.Notes, "Simulation APT finale non vérifiable.")
	} else {
		v.Remaining = remaining
		if len(remaining) > 0 {
			v.Notes = append(v.Notes, "Des mises à jour APT restent proposées.")
		}
	}
	out, err := e.guest(id, []string{"systemctl", "list-units", "--type=service", "--state=failed", "--no-legend", "--no-pager", "--plain"}, false)
	if err != nil {
		v.Notes = append(v.Notes, "État des services non vérifiable.")
	} else {
		for _, row := range lines(out) {
			v.FailedServices = append(v.FailedServices, strings.Fields(row)[0])
		}
		if len(v.FailedServices) > 0 {
			v.Notes = append(v.Notes, "Services en échec observés ; état antérieur non comparé.")
		}
	}
	return v
}
func (e *Engine) journal(id, event string) error {
	path := filepath.Join(e.StateDir, "journal.jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return errors.New("journal privé indisponible")
	}
	defer f.Close()
	if err = f.Chmod(0600); err != nil {
		return err
	}
	raw, _ := json.Marshal(map[string]any{"date": time.Now().UTC().Format(time.RFC3339), "target": id, "event": event, "demo": e.Demo})
	_, err = f.Write(append(raw, '\n'))
	return err
}
func (e *Engine) lock() (func() error, error) {
	if err := os.MkdirAll(e.StateDir, 0700); err != nil {
		return nil, err
	}
	if err := os.Chmod(e.StateDir, 0700); err != nil {
		return nil, err
	}
	path := filepath.Join(e.StateDir, "apply.lock")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return nil, errors.New("verrou local actif ou inaccessible")
	}
	fmt.Fprintf(f, "pid=%d\n", os.Getpid())
	f.Close()
	if !e.Demo {
		if _, err = e.run([]string{"mkdir", "/run/lock/homelab-maintainer.apply"}, true); err != nil {
			os.Remove(path)
			return nil, errors.New("verrou global actif ou inaccessible")
		}
	}
	return func() error {
		var remoteErr error
		if !e.Demo {
			_, remoteErr = e.run([]string{"rmdir", "/run/lock/homelab-maintainer.apply"}, true)
		}
		localErr := os.Remove(path)
		if remoteErr != nil {
			return remoteErr
		}
		return localErr
	}, nil
}
func (e *Engine) Apply(p Plan, backup string) (result Result, err error) {
	result = Result{Target: p.Target, Demo: e.Demo, Notes: []string{}, Verification: Verification{Versions: []VersionCheck{}, Remaining: []Package{}, FailedServices: []string{}, Notes: []string{}}}
	if _, err = e.find(p.Target, true, true); err != nil {
		return result, err
	}
	if !storageRE.MatchString(backup) {
		return result, errors.New("stockage de sauvegarde invalide")
	}
	if p.Demo != e.Demo || p.Fingerprint != fingerprint(p) {
		return result, errors.New("plan altéré ou mode différent")
	}
	unlock, err := e.lock()
	if err != nil {
		return result, err
	}
	defer func() {
		if releaseErr := unlock(); err == nil && releaseErr != nil {
			err = errors.New("libération du verrou impossible : vérifier son état")
		}
	}()
	if err = e.journal(p.Target, "start"); err != nil {
		return result, err
	}
	defer func() {
		if err != nil {
			_ = e.journal(p.Target, "failed")
		}
	}()
	current, err := e.Plan(p.Target)
	if err != nil {
		return result, err
	}
	if current.Fingerprint != p.Fingerprint {
		return result, errors.New("identité ou simulation modifiée : recréer le plan")
	}
	if len(current.Packages) == 0 {
		result.Status = "nothing-to-do"
		err = e.journal(p.Target, result.Status)
		return result, err
	}
	if !e.Demo {
		if err = e.preflight(p.Target); err != nil {
			return result, err
		}
		if _, err = e.run([]string{"vzdump", strings.TrimPrefix(p.Target, "qVM"), "--storage", backup, "--mode", "snapshot", "--remove", "0"}, true); err != nil {
			return result, err
		}
	}
	if err = e.journal(p.Target, "backup-success"); err != nil {
		return result, err
	}
	if _, err = e.find(p.Target, true, true); err != nil {
		return result, err
	}
	current, err = e.Plan(p.Target)
	if err != nil {
		return result, err
	}
	if current.Fingerprint != p.Fingerprint {
		return result, errors.New("dérive après sauvegarde : application refusée")
	}
	if !e.Demo {
		_, err = e.guest(p.Target, []string{"env", "DEBIAN_FRONTEND=noninteractive", "apt-get", "-y", "-o", "Dpkg::Options::=--force-confdef", "-o", "Dpkg::Options::=--force-confold", "--no-remove", "upgrade"}, true)
		if err != nil {
			return result, err
		}
	}
	if err = e.journal(p.Target, "upgrade-success"); err != nil {
		return result, err
	}
	if !e.Demo {
		result.Verification = e.verify(p.Target, p.Packages)
	}
	result.Status = "applied"
	if len(result.Verification.Notes) > 0 {
		result.Status = "needs-review"
	}
	result.Notes = append([]string{"Sauvegarde conservée ; observations techniques, accès et UX non validés."}, result.Verification.Notes...)
	if strings.HasPrefix(p.Target, "qVM") {
		result.Notes = append(result.Notes, "VM sans QGA : sauvegarde snapshot potentiellement crash-consistent ; restauration applicative non testée.")
	}
	err = e.journal(p.Target, result.Status)
	return result, err
}

type catalogueEntry struct {
	id, name string
	packages []string
}

var catalogue = []catalogueEntry{{"plex", "Plex", []string{"plexmediaserver"}}, {"jellyfin", "Jellyfin", []string{"jellyfin", "jellyfin-server"}}, {"netbird", "NetBird", []string{"netbird"}}, {"docker", "Docker", []string{"docker-ce", "docker.io"}}}

func demoPackages() []Package {
	return []Package{{"curl", "8.14.1-1", "8.14.1-2"}, {"openssl", "3.5.1-1", "3.5.1-2"}}
}
func demoTargets() []Target {
	return []Target{{"host", "host", "PVE-demo", "running", "demo-host"}, {"101", "ct", "lab-web", "running", "demo-101"}, {"102", "ct", "lab-docker", "running", "demo-102"}, {"103", "ct", "lab-tools", "running", "demo-103"}, {"104", "ct", "lab-offline", "stopped", "demo-104"}, {"qVM201", "vm", "debian-demo", "running", "demo-vm201"}}
}
