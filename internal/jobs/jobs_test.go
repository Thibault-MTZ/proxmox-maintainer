package jobs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRejectUnknownActionBeforeConnection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "request.json")
	data, _ := json.Marshal(Request{Action: "unknown"})
	os.WriteFile(path, data, 0600)
	if Run(path) == nil {
		t.Fatal("unknown action accepted")
	}
}
func TestMalformedRequest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "request.json")
	os.WriteFile(path, []byte("bad"), 0600)
	if Run(path) == nil {
		t.Fatal("bad JSON accepted")
	}
}
