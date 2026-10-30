package main

import (
	"encoding/json"
	"fmt"
	"github.com/Thibault-MTZ/proxmox-maintainer/internal/config"
	"github.com/Thibault-MTZ/proxmox-maintainer/internal/engine"
	"github.com/Thibault-MTZ/proxmox-maintainer/internal/jobs"
	"github.com/Thibault-MTZ/proxmox-maintainer/internal/tui"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
	"os"
)

var version = "0.2.0-beta.1-dev"
var defaultAdvanced = "false"

func main() {
	if err := root().Execute(); err != nil {
		os.Exit(1)
	}
}
func root() *cobra.Command {
	var demo, advanced bool
	var host, dir string
	var vmSSH map[string]string
	load := func() (config.Config, *engine.Engine, error) {
		c, err := config.Load(dir)
		if err != nil {
			return c, nil, err
		}
		if host != "" {
			c.Host = host
		}
		if len(vmSSH) > 0 {
			c.VMSSH = vmSSH
		}
		e := engine.New(demo, c.Host, dir)
		e.VMSSH = c.VMSSH
		return c, e, nil
	}
	cmd := &cobra.Command{Use: "proxmox-maint", Short: "TUI bêta de mise à jour Proxmox CT / VM Linux", Version: version, SilenceUsage: true}
	cmd.PersistentFlags().BoolVar(&demo, "demo", false, "Invités fictifs, aucune connexion serveur")
	cmd.PersistentFlags().BoolVar(&advanced, "advanced", defaultAdvanced == "true", "Découverte expérimentale applications/services/Docker")
	cmd.PersistentFlags().StringVar(&host, "host", "", "Alias SSH du PVE (vide = local)")
	cmd.PersistentFlags().StringVar(&dir, "state-dir", config.DefaultDir(), "Configuration, plans et journaux")
	cmd.PersistentFlags().StringToStringVar(&vmSSH, "vm-ssh", nil, "VMID=alias SSH accessible DEPUIS PVE, ex. qVM201=debian-lab")
	cmd.RunE = func(_ *cobra.Command, _ []string) error {
		c, e, err := load()
		if err != nil {
			return err
		}
		m := tui.New(e, c, dir).WithAdvanced(advanced)
		_, err = tea.NewProgram(m, tea.WithAltScreen()).Run()
		return err
	}
	scan := &cobra.Command{Use: "scan", Short: "Inventaire JSON, sans modification", RunE: func(_ *cobra.Command, _ []string) error {
		_, e, err := load()
		if err != nil {
			return err
		}
		t, err := e.Inventory()
		if err != nil {
			return err
		}
		return printJSON(t)
	}}
	plan := &cobra.Command{Use: "check ID", Short: "Simulation APT d’un invité actif", Args: cobra.ExactArgs(1), RunE: func(_ *cobra.Command, args []string) error {
		_, e, err := load()
		if err != nil {
			return err
		}
		p, err := e.Plan(args[0])
		if err != nil {
			return err
		}
		return printJSON(p)
	}}
	worker := &cobra.Command{Use: "worker REQUEST", Hidden: true, Args: cobra.ExactArgs(1), RunE: func(_ *cobra.Command, args []string) error { return jobs.Run(args[0]) }}
	cmd.AddCommand(scan, plan, worker)
	return cmd
}
func printJSON(value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}
