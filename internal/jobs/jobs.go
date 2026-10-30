// Package jobs runs approved maintenance separately from the terminal process.
package jobs

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/Thibault-MTZ/proxmox-maintainer/internal/config"
	"github.com/Thibault-MTZ/proxmox-maintainer/internal/engine"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

type Request struct {
	Action   string        `json:"action"`
	Config   config.Config `json:"config"`
	Plans    []engine.Plan `json:"plans,omitempty"`
	IDs      []string      `json:"ids,omitempty"`
	StateDir string        `json:"state_dir"`
}
type Outcome struct {
	Finished bool            `json:"finished"`
	Failed   bool            `json:"failed"`
	Results  []engine.Result `json:"results"`
	Error    string          `json:"error,omitempty"`
}

func Start(r Request) (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(r.StateDir, "runs", time.Now().Format("20060102-150405.000000000"))
	if err = os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "request.json")
	if err = config.SaveJSON(path, r); err != nil {
		return "", err
	}
	log, err := os.OpenFile(filepath.Join(dir, "worker.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return "", err
	}
	defer log.Close()
	command := exec.Command(exe, "worker", path)
	command.Stdout = log
	command.Stderr = log
	if err = detach(command); err != nil {
		return "", err
	}
	return dir, nil
}
func Run(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var r Request
	if err = json.Unmarshal(data, &r); err != nil {
		return err
	}
	if r.Action != "apply" && r.Action != "refresh" {
		return errors.New("action inconnue")
	}
	e := engine.New(false, r.Config.Host, r.StateDir)
	e.VMSSH = r.Config.VMSSH
	outcome := Outcome{}
	resultPath := filepath.Join(filepath.Dir(path), "result.json")
	fail := func(err error) error {
		outcome.Failed = true
		outcome.Finished = true
		outcome.Error = err.Error()
		config.SaveJSON(resultPath, outcome)
		return err
	}
	if r.Action == "apply" {
		for _, plan := range r.Plans {
			fmt.Printf("Cible %s : sauvegarde et mise à jour\n", plan.Target)
			result, err := e.Apply(plan, r.Config.BackupStorage)
			if err != nil {
				return fail(err)
			}
			outcome.Results = append(outcome.Results, result)
			if err = config.SaveJSON(resultPath, outcome); err != nil {
				return err
			}
			if result.Status == "needs-review" {
				return fail(errors.New("contrôle final à examiner ; lot interrompu"))
			}
		}
	} else {
		for _, id := range r.IDs {
			if err = e.Refresh(id); err != nil {
				return fail(err)
			}
		}
	}
	outcome.Finished = true
	return config.SaveJSON(resultPath, outcome)
}
