// Package screenshot manages a disk-backed cache of device screenshots keyed by composable FQN.
// Multiple instances of compose-preview share the same cache on disk.
package screenshot

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// SharedDir is the directory where screenshots are stored.
var SharedDir = filepath.Join(os.TempDir(), "compose-preview")

// Entry is a cached screenshot.
type Entry struct {
	PNGData    []byte
	CapturedAt time.Time
}

// Age returns a human-readable age string.
func (e *Entry) Age() string {
	d := time.Since(e.CapturedAt)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	default:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
}

// Cache stores screenshots on disk keyed by FQN.
type Cache struct {
	dir string
}

// NewCache creates a new disk-backed screenshot cache.
func NewCache() *Cache {
	dir := filepath.Join(SharedDir, "screenshots")
	os.MkdirAll(dir, 0755)
	return &Cache{dir: dir}
}

// fqnToFile converts a FQN to a safe filename.
func fqnToFile(fqn string) string {
	h := sha256.Sum256([]byte(fqn))
	return fmt.Sprintf("%x.png", h[:8])
}

// Get returns the cached screenshot for the given FQN, or nil if not found.
func (c *Cache) Get(fqn string) *Entry {
	path := filepath.Join(c.dir, fqnToFile(fqn))
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil
	}
	return &Entry{
		PNGData:    data,
		CapturedAt: info.ModTime(),
	}
}

// Put stores a screenshot for the given FQN on disk and signals external viewers.
func (c *Cache) Put(fqn string, pngData []byte) {
	path := filepath.Join(c.dir, fqnToFile(fqn))
	os.WriteFile(path, pngData, 0644)
	writeShared(fqn, pngData)
}

// SignalSelection writes state.json to notify external viewers of the current selection.
func (c *Cache) SignalSelection(fqn string) {
	entry := c.Get(fqn)
	hasScreenshot := entry != nil
	age := ""
	if hasScreenshot {
		age = entry.Age()
		writeShared(fqn, entry.PNGData)
	}

	state := map[string]interface{}{
		"fqn":           fqn,
		"hasScreenshot": hasScreenshot,
		"age":           age,
		"timestamp":     time.Now().UnixMilli(),
	}
	os.MkdirAll(SharedDir, 0755)
	data, _ := json.Marshal(state)
	os.WriteFile(filepath.Join(SharedDir, "state.json"), data, 0644)
}

// SignalCapturing writes state.json with capturing=true.
func (c *Cache) SignalCapturing(fqn string) {
	state := map[string]interface{}{
		"fqn":           fqn,
		"hasScreenshot": false,
		"capturing":     true,
		"timestamp":     time.Now().UnixMilli(),
	}
	os.MkdirAll(SharedDir, 0755)
	data, _ := json.Marshal(state)
	os.WriteFile(filepath.Join(SharedDir, "state.json"), data, 0644)
}

func writeShared(fqn string, pngData []byte) {
	os.MkdirAll(SharedDir, 0755)
	os.WriteFile(filepath.Join(SharedDir, "current.png"), pngData, 0644)
	now := time.Now()
	state := map[string]interface{}{
		"fqn":           fqn,
		"hasScreenshot": true,
		"capturing":     false,
		"capturedAt":    now.Format("15:04:05"),
		"timestamp":     now.UnixMilli(),
	}
	data, _ := json.Marshal(state)
	os.WriteFile(filepath.Join(SharedDir, "state.json"), data, 0644)
}
