package gameimport

import (
	"path/filepath"
	"testing"
)

// TestWantedPath covers the stable resource selection contract.
func TestWantedPath(t *testing.T) {
	var cases map[string]bool = map[string]bool{
		"content/GameParams.data":                   true,
		"scripts/entities.xml":                      true,
		"scripts/entity_defs/Account.def":           true,
		"gui/battle_hud/markers/ship/destroyer.png": true,
		"spaces/islands/minimap.png":                true,
		"spaces/islands/unrelated.geometry":         false,
		"content/gameplay/islands/space.settings":   true,
		"content/gameplay/islands/terrain.visual":   false,
		"vehicles/american/ship/model.visual":       false,
	}

	for path, expected := range cases {
		var actual bool

		if actual = wantedPath(path); actual != expected {
			t.Errorf("wantedPath(%q) = %t, expected %t", path, actual, expected)
		}
	}
}

// TestSafeDestination rejects archive traversal while accepting VFS paths.
func TestSafeDestination(t *testing.T) {
	var (
		destination string
		err         error
		root        string = t.TempDir()
	)

	if destination, err = safeDestination(root, "gui/icons/ship.png"); err != nil {
		t.Fatal(err)
	}

	if destination != filepath.Join(root, "gui", "icons", "ship.png") {
		t.Fatalf("unexpected destination %q", destination)
	}

	for _, path := range []string{"../escape", "gui/../../escape"} {
		if _, err = safeDestination(root, path); err == nil {
			t.Errorf("safeDestination accepted %q", path)
		}
	}
}
