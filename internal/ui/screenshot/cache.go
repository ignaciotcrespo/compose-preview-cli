// Package screenshot manages a cache of device screenshots keyed by composable FQN.
package screenshot

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// SharedDir is the directory where screenshots are written for external viewers.
var SharedDir = filepath.Join(os.TempDir(), "compose-preview")

// Entry is a cached screenshot.
type Entry struct {
	PNGData   []byte
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

// Cache stores screenshots keyed by composable FQN.
type Cache struct {
	mu      sync.RWMutex
	entries map[string]*Entry
}

// NewCache creates a new screenshot cache.
func NewCache() *Cache {
	return &Cache{entries: make(map[string]*Entry)}
}

// Get returns the cached screenshot for the given FQN, or nil if not found.
func (c *Cache) Get(fqn string) *Entry {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.entries[fqn]
}

// Put stores a screenshot for the given FQN and writes it to the shared directory.
func (c *Cache) Put(fqn string, pngData []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[fqn] = &Entry{
		PNGData:    pngData,
		CapturedAt: time.Now(),
	}
	// Write to shared dir for external viewers (Electron)
	writeShared(fqn, pngData)
}

// SignalSelection writes a state.json to notify external viewers of the current selection.
// Called when navigating previews (even without a new screenshot).
func (c *Cache) SignalSelection(fqn string) {
	entry := c.Get(fqn)
	hasScreenshot := entry != nil
	age := ""
	if hasScreenshot {
		age = entry.Age()
		// Also write the cached PNG so Electron can show it
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

// SignalCapturing writes state.json with capturing=true to show loading state in Electron.
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
