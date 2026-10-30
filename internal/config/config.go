// Package config stores user choices separately from program releases.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Config struct {
	Host          string            `json:"host"`
	BackupStorage string            `json:"backup_storage"`
	VMSSH         map[string]string `json:"vm_ssh,omitempty"`
}

func DefaultDir() string {
	if path := os.Getenv("HOMELAB_MAINT_STATE"); path != "" {
		return path
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "state", "homelab-maintainer")
}

func Load(dir string) (Config, error) {
	var c Config
	data, err := os.ReadFile(filepath.Join(dir, "config.json"))
	if os.IsNotExist(err) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	err = json.Unmarshal(data, &c)
	return c, err
}

func SaveJSON(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".maint-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), path)
}

func Save(dir string, c Config) error { return SaveJSON(filepath.Join(dir, "config.json"), c) }
