package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	c := Config{Host: "pve-test", BackupStorage: "BACKUP"}
	if err := Save(dir, c); err != nil {
		t.Fatal(err)
	}
	got, err := Load(dir)
	if err != nil || !reflect.DeepEqual(got, c) {
		t.Fatalf("%+v %v", got, err)
	}
	info, _ := os.Stat(filepath.Join(dir, "config.json"))
	if info.Mode().Perm() != 0600 {
		t.Fatal("permissions")
	}
}
func TestMissingAndMalformed(t *testing.T) {
	dir := t.TempDir()
	if _, err := Load(dir); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "config.json"), []byte("broken"), 0600)
	if _, err := Load(dir); err == nil {
		t.Fatal("malformed config accepted")
	}
}
