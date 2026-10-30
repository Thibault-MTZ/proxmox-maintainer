package engine

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const simulation = "Reading package lists...\nInst curl [8.1-1] (8.1-2 Debian:stable [amd64])\nConf curl (8.1-2 Debian:stable [amd64])\n"

type call struct {
	args     []string
	mutation bool
}
type fake struct {
	calls                                                                                                                                                                                    []call
	backupFail, backedUp, upgraded, drift, afterDrift, badMachine, lowSpace, auditFail, checkFail, versionMismatch, remaining, failedServices, locked, template, rootfsDrift, postHealthFail bool
}

func (f *fake) run(args []string, mutation bool) (string, error) {
	f.calls = append(f.calls, call{append([]string{}, args...), mutation})
	cmd := args
	if args[0] == "ssh" {
		// Decode the engine's quoted fixed commands without executing any shell.
		matches := regexp.MustCompile(`'([^']*)'`).FindAllStringSubmatch(args[len(args)-1], -1)
		cmd = []string{}
		for _, m := range matches {
			cmd = append(cmd, m[1])
		}
		if len(cmd) < 3 {
			return "", errors.New("invalid mocked SSH command")
		}
		cmd = cmd[2:]
	}
	if args[0] == "pct" && len(args) > 2 && args[1] == "exec" {
		cmd = args[6:]
	}
	key := strings.Join(cmd, " ")
	switch {
	case key == "hostname":
		return "pve-test\n", nil
	case key == "pct list":
		return "VMID Status Lock Name\n102 running dns\n103 stopped off\n", nil
	case key == "qm list":
		return "VMID NAME STATUS MEM BOOTDISK PID\n104 vm running 2048 10 999\n", nil
	case len(args) > 1 && args[0] == "qm" && args[1] == "config":
		s := "name: vm\nscsi0: local-lvm:vm-104-disk-0,size=16G\nboot: order=scsi0\nsmbios1: uuid=01234567-89ab-cdef-0123-456789abcdef\n"
		if f.rootfsDrift {
			s = strings.ReplaceAll(s, "disk-0", "disk-1")
		}
		if f.locked {
			s += "lock: backup\n"
		}
		if f.template {
			s += "template: 1\n"
		}
		return s, nil
	case len(args) > 1 && args[0] == "pct" && args[1] == "config":
		s := "hostname: dns\nrootfs: local-lvm:vm-102-disk-0,size=8G\n"
		if f.rootfsDrift {
			s = strings.ReplaceAll(s, "disk-0", "disk-1")
		}
		if f.locked {
			s += "lock: backup\n"
		}
		if f.template {
			s += "template: 1\n"
		}
		return s, nil
	case key == "cat /etc/machine-id":
		if f.badMachine {
			return strings.Repeat("b", 32) + "\n", nil
		}
		return strings.Repeat("a", 32) + "\n", nil
	case key == "cat /etc/os-release":
		return "PRETTY_NAME=\"Debian GNU/Linux 13\"\n", nil
	case strings.HasPrefix(key, "apt-get -s"):
		if f.upgraded && !f.remaining {
			return "", nil
		}
		if f.drift || (f.afterDrift && f.backedUp) {
			return strings.ReplaceAll(simulation, "8.1-2", "8.1-3"), nil
		}
		return simulation, nil
	case args[0] == "vzdump":
		f.backedUp = true
		if f.backupFail {
			return "", errors.New("private repository error: secret")
		}
		return "backup successful", nil
	case strings.HasPrefix(key, "dpkg-query"):
		version := "8.1-1"
		if f.upgraded && !f.versionMismatch {
			version = "8.1-2"
		}
		return "jellyfin\t10.10.7\tii \ncurl\t" + version + "\tii \n", nil
	case key == "df -Pk /":
		if f.lowSpace {
			return "Filesystem 1024-blocks Used Available Capacity Mounted on\n/dev/root 2000000 1500000 524287 90% /\n", nil
		}
		return "Filesystem 1024-blocks Used Available Capacity Mounted on\n/dev/root 2000000 100000 1048576 10% /\n", nil
	case key == "dpkg --audit":
		if f.auditFail || (f.upgraded && f.postHealthFail) {
			return "incomplete package", nil
		}
		return "", nil
	case key == "apt-get check":
		if f.checkFail {
			return "", errors.New("dependency failure password=secret")
		}
		return "", nil
	case strings.Contains(key, "DEBIAN_FRONTEND=noninteractive"):
		f.upgraded = true
		return "", nil
	case strings.HasPrefix(key, "docker ps"):
		return "media\tjellyfin/jellyfin:stable\n", nil
	case strings.HasPrefix(key, "systemctl"):
		if strings.Contains(key, "--state=failed") {
			if f.failedServices {
				return "broken.service loaded failed failed Broken\n", nil
			}
			return "", nil
		}
		return "ssh.service loaded active running SSH\n", nil
	case strings.HasPrefix(key, "ss "):
		return "tcp LISTEN 0 128 *:22 *:*\n", nil
	case strings.HasPrefix(key, "find "):
		return "/etc/apt/sources.list.d/debian.sources\n", nil
	}
	return "", nil
}
func setup(t *testing.T) (*Engine, *fake) {
	t.Helper()
	f := &fake{}
	e := New(false, "pve.test", t.TempDir())
	e.Exec = f.run
	return e, f
}
func plan(t *testing.T, e *Engine) Plan {
	t.Helper()
	p, err := e.Plan("102")
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func (f *fake) has(command string) bool {
	for _, c := range f.calls {
		if c.args[0] == command {
			return true
		}
	}
	return false
}
func TestInventoryAndPassiveTargets(t *testing.T) {
	e, f := setup(t)
	targets, err := e.Inventory()
	if err != nil || len(targets) != 4 || targets[3].ID != "qVM104" {
		t.Fatalf("inventory %+v %v", targets, err)
	}
	for _, id := range []string{"103", "qVM104"} {
		before := len(f.calls)
		r, err := e.Inspect(id)
		if err != nil || len(r.Notes) == 0 {
			t.Fatal(r, err)
		}
		for _, c := range f.calls[before:] {
			if len(c.args) > 1 && c.args[1] == "exec" {
				t.Fatal("entered passive guest")
			}
		}
	}
}
func TestApplyBackupOrderAndFlags(t *testing.T) {
	e, f := setup(t)
	p := plan(t, e)
	r, err := e.Apply(p, "NAS-NFS")
	if err != nil || r.Status != "applied" {
		t.Fatal(r, err)
	}
	backup, upgrade := -1, -1
	for i, c := range f.calls {
		key := strings.Join(c.args, " ")
		if c.args[0] == "vzdump" {
			backup = i
			if !strings.Contains(key, "--mode snapshot --remove 0") || !c.mutation {
				t.Fatal(c)
			}
		}
		if strings.Contains(key, "DEBIAN_FRONTEND=noninteractive") {
			upgrade = i
			if !strings.Contains(key, "--no-remove upgrade") || !strings.Contains(key, "Dpkg::Options::=--force-confold") || !c.mutation {
				t.Fatal(c)
			}
		}
	}
	if backup < 0 || upgrade <= backup {
		t.Fatalf("backup=%d upgrade=%d", backup, upgrade)
	}
	if len(r.Verification.Versions) != 1 || !r.Verification.Versions[0].Matches {
		t.Fatal(r.Verification)
	}
	if _, err = os.Stat(filepath.Join(e.StateDir, "apply.lock")); !os.IsNotExist(err) {
		t.Fatal("lock not released")
	}
	raw, err := os.ReadFile(filepath.Join(e.StateDir, "journal.jsonl"))
	if err != nil || strings.Contains(string(raw), "secret") {
		t.Fatal("journal invalid", err)
	}
}
func TestBackupFailureBlocksAndRedacts(t *testing.T) {
	e, f := setup(t)
	p := plan(t, e)
	f.backupFail = true
	_, err := e.Apply(p, "NAS-NFS")
	if err == nil || strings.Contains(err.Error(), "secret") || f.upgraded || !f.has("rmdir") {
		t.Fatal("backup guard failed", err)
	}
}
func TestDriftBeforeAndAfterBackup(t *testing.T) {
	for _, variant := range []string{"packages", "machine", "rootfs", "after"} {
		t.Run(variant, func(t *testing.T) {
			e, f := setup(t)
			p := plan(t, e)
			switch variant {
			case "packages":
				f.drift = true
			case "machine":
				f.badMachine = true
			case "rootfs":
				f.rootfsDrift = true
			case "after":
				f.afterDrift = true
			}
			_, err := e.Apply(p, "NAS-NFS")
			if err == nil || f.upgraded {
				t.Fatal("drift accepted", err)
			}
			if variant != "after" && f.backedUp {
				t.Fatal("unexpected backup")
			}
			if variant == "after" && !f.backedUp {
				t.Fatal("backup not reached")
			}
		})
	}
}
func TestRejectHostVMStoppedAndInvalidIDs(t *testing.T) {
	e, f := setup(t)
	for _, id := range []string{"host", "qVM104", "103", "102;reboot", "0", "-1"} {
		if _, err := e.Apply(Plan{Target: id}, "NAS-NFS"); err == nil {
			t.Fatal("accepted", id)
		}
		if err := e.Refresh(id); err == nil {
			t.Fatal("refreshed", id)
		}
	}
	if f.upgraded || f.backedUp {
		t.Fatal("mutation occurred")
	}
}
func TestLockedAndTemplateCT(t *testing.T) {
	for _, template := range []bool{false, true} {
		e, f := setup(t)
		if template {
			f.template = true
		} else {
			f.locked = true
		}
		if _, err := e.Plan("102"); err == nil {
			t.Fatal("unsafe CT planned")
		}
		if err := e.Refresh("102"); err == nil {
			t.Fatal("unsafe CT refreshed")
		}
		for _, c := range f.calls {
			if len(c.args) > 1 && c.args[1] == "exec" {
				t.Fatal("entered unsafe CT")
			}
		}
	}
}
func TestAlteredPlanStorageAndMode(t *testing.T) {
	for _, variant := range []string{"plan", "storage", "mode"} {
		e, f := setup(t)
		p := plan(t, e)
		storage := "NAS-NFS"
		switch variant {
		case "plan":
			p.Packages[0].Candidate = "bad"
		case "storage":
			storage = "NAS;reboot"
		case "mode":
			p.Demo = true
		}
		if _, err := e.Apply(p, storage); err == nil {
			t.Fatal("accepted", variant)
		}
		if f.backedUp || f.upgraded {
			t.Fatal("mutated")
		}
	}
}
func TestParseSimulationDefensive(t *testing.T) {
	p, err := ParseSimulation(simulation)
	if err != nil || len(p) != 1 || p[0].Candidate != "8.1-2" {
		t.Fatal(p, err)
	}
	for _, raw := range []string{"Remv libc6 [1]", "Purg curl [1]", "Inst bad;reboot [1] (2 Debian)", "Inst curl ???", "Inst curl [1] (2;reboot Debian)", simulation + simulation} {
		if _, err := ParseSimulation(raw); err == nil {
			t.Fatal("accepted", raw)
		}
	}
}
func TestPreflightGuards(t *testing.T) {
	for _, which := range []string{"space", "audit", "check"} {
		t.Run(which, func(t *testing.T) {
			e, f := setup(t)
			p := plan(t, e)
			switch which {
			case "space":
				f.lowSpace = true
			case "audit":
				f.auditFail = true
			case "check":
				f.checkFail = true
			}
			if _, err := e.Apply(p, "NAS-NFS"); err == nil {
				t.Fatal("preflight accepted")
			}
			if f.backedUp || f.upgraded {
				t.Fatal("mutated")
			}
		})
	}
}
func TestPostflightNeedsReview(t *testing.T) {
	for _, which := range []string{"versions", "remaining", "services", "health"} {
		t.Run(which, func(t *testing.T) {
			e, f := setup(t)
			p := plan(t, e)
			switch which {
			case "versions":
				f.versionMismatch = true
			case "remaining":
				f.remaining = true
			case "services":
				f.failedServices = true
			case "health":
				f.postHealthFail = true
			}
			r, err := e.Apply(p, "NAS-NFS")
			if err != nil || r.Status != "needs-review" || len(r.Verification.Notes) == 0 {
				t.Fatal(r, err)
			}
		})
	}
}
func TestInspectVersionsAndNoSecretsCommands(t *testing.T) {
	e, f := setup(t)
	r, err := e.Inspect("102")
	if err != nil || r.OS != "Debian GNU/Linux 13" {
		t.Fatal(r, err)
	}
	found := false
	for _, app := range r.Apps {
		if app.ID == "jellyfin" {
			found = app.Versions["jellyfin"] == "10.10.7"
		}
	}
	if !found {
		t.Fatal("app version absent", r.Apps)
	}
	if _, err = json.Marshal(r); err != nil {
		t.Fatal(err)
	}
	for _, c := range f.calls {
		for _, a := range c.args {
			if a == "inspect" || a == "printenv" {
				t.Fatal("secret inspection")
			}
		}
	}
}
func TestDemoNeverExecutes(t *testing.T) {
	e := New(true, "", t.TempDir())
	e.Exec = func([]string, bool) (string, error) { t.Fatal("demo subprocess"); return "", nil }
	if _, err := e.Inventory(); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Inspect("102"); err != nil {
		t.Fatal(err)
	}
	if err := e.Refresh("102"); err != nil {
		t.Fatal(err)
	}
	r, err := e.Apply(plan(t, e), "NAS-NFS")
	if err != nil || r.Status != "applied" || !r.Demo {
		t.Fatal(r, err)
	}
}
func TestExistingLocalLockBlocks(t *testing.T) {
	e, f := setup(t)
	p := plan(t, e)
	if err := os.WriteFile(filepath.Join(e.StateDir, "apply.lock"), []byte("existing"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Apply(p, "NAS-NFS"); err == nil {
		t.Fatal("lock bypass")
	}
	if f.backedUp {
		t.Fatal("mutated")
	}
}
func TestSSHValidationAndQuoting(t *testing.T) {
	e := New(false, "bad;host", t.TempDir())
	if _, err := e.execute([]string{"hostname"}, false); err == nil {
		t.Fatal("unsafe host accepted")
	}
	if shellQuote("x'y") != "'x'\"'\"'y'" {
		t.Fatal("quote incorrect")
	}
}
func TestRefreshMutationClassification(t *testing.T) {
	e, f := setup(t)
	if err := e.Refresh("102"); err != nil {
		t.Fatal(err)
	}
	last := f.calls[len(f.calls)-1]
	if !last.mutation || !strings.Contains(strings.Join(last.args, " "), "apt-get update") {
		t.Fatal(last)
	}
}

func TestVMExplicitSSHRouteAndNumericBackup(t *testing.T) {
	e, f := setup(t)
	e.VMSSH["qVM104"] = "vm-lab"
	p, err := e.Plan("qVM104")
	if err != nil {
		t.Fatal(err)
	}
	r, err := e.Apply(p, "NAS-NFS")
	if err != nil || r.Status != "applied" {
		t.Fatal(r, err)
	}
	sshSeen, backupSeen := false, false
	for _, c := range f.calls {
		if c.args[0] == "ssh" {
			sshSeen = true
			if c.args[5] != "root@vm-lab" {
				t.Fatal("unexpected SSH destination", c.args)
			}
			if !strings.Contains(c.args[len(c.args)-1], "'LC_ALL=C'") {
				t.Fatal("guest locale not set")
			}
		}
		if c.args[0] == "vzdump" {
			backupSeen = true
			if c.args[1] != "104" {
				t.Fatal("backup VM id not numeric", c.args)
			}
		}
		for _, a := range c.args {
			if a == "agent" || a == "guest" {
				t.Fatal("unexpected QGA use")
			}
		}
	}
	if !sshSeen || !backupSeen {
		t.Fatal("missing SSH or backup")
	}
	if !strings.Contains(strings.Join(r.Notes, " "), "crash-consistent") {
		t.Fatal("backup limitation absent")
	}
	if err = e.Refresh("qVM104"); err != nil {
		t.Fatal(err)
	}
	raw, err := e.Inspect("qVM104")
	if err != nil || raw.OS != "Debian GNU/Linux 13" {
		t.Fatal(raw, err)
	}
}

func TestVMUnconfiguredOrUnsafeAliasBlocked(t *testing.T) {
	for _, alias := range []string{"", "bad;reboot", "root@vm", "-vm"} {
		e, f := setup(t)
		e.VMSSH["qVM104"] = alias
		if _, err := e.Plan("qVM104"); err == nil {
			t.Fatal("accepted alias", alias)
		}
		if err := e.Refresh("qVM104"); err == nil {
			t.Fatal("refreshed unsafe alias")
		}
		r, err := e.Inspect("qVM104")
		if err != nil || len(r.Notes) == 0 {
			t.Fatal(r, err)
		}
		if f.has("ssh") || f.backedUp || f.upgraded {
			t.Fatal("unsafe VM execution")
		}
	}
}

func TestVMIdentityAndLockGuards(t *testing.T) {
	for _, variant := range []string{"alias", "machine", "disk", "lock", "template", "after"} {
		t.Run(variant, func(t *testing.T) {
			e, f := setup(t)
			e.VMSSH["qVM104"] = "vm-lab"
			p, err := e.Plan("qVM104")
			if err != nil {
				t.Fatal(err)
			}
			switch variant {
			case "alias":
				e.VMSSH["qVM104"] = "other-vm"
			case "machine":
				f.badMachine = true
			case "disk":
				f.rootfsDrift = true
			case "lock":
				f.locked = true
			case "template":
				f.template = true
			case "after":
				f.afterDrift = true
			}
			if _, err = e.Apply(p, "NAS-NFS"); err == nil {
				t.Fatal("unsafe VM plan accepted")
			}
			if f.upgraded {
				t.Fatal("unsafe VM upgraded")
			}
			if variant != "after" && f.backedUp {
				t.Fatal("unexpected backup")
			}
		})
	}
}

func TestDemoVMNoSSHAndGenericNames(t *testing.T) {
	e := New(true, "", t.TempDir())
	e.Exec = func([]string, bool) (string, error) { t.Fatal("demo executed subprocess"); return "", nil }
	list, err := e.Inventory()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"101": "lab-web", "102": "lab-docker", "103": "lab-tools", "104": "lab-offline", "qVM201": "debian-demo"}
	for _, target := range list {
		if name, ok := want[target.ID]; ok && target.Name != name {
			t.Fatal(target)
		}
	}
	p, err := e.Plan("qVM201")
	if err != nil {
		t.Fatal(err)
	}
	r, err := e.Apply(p, "NAS-NFS")
	if err != nil || !r.Demo {
		t.Fatal(r, err)
	}
	if _, err = e.Inspect("qVM201"); err != nil {
		t.Fatal(err)
	}
	if err = e.Refresh("qVM201"); err != nil {
		t.Fatal(err)
	}
}
