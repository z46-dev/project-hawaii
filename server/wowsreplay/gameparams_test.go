package wowsreplay

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoadShipParameters verifies imported GameParams can provide real ship classes.
func TestLoadShipParameters(t *testing.T) {
	var (
		bundle      *ResourceBundle
		borders     map[uint64]MapBorderParameters
		border      MapBorderParameters
		borderFound bool
		err         error
		found       bool
		root        string = filepath.Join("testdata", "game", "15.6.0_12830008")
		ship        ShipParameters
		ships       map[uint64]ShipParameters
	)

	if _, err = os.Stat(root); os.IsNotExist(err) {
		t.Skip("imported 15.6 resource fixture is not present")
	}

	if bundle, err = OpenResourceBundle(root); err != nil {
		t.Fatal(err)
	}

	if ships, err = bundle.LoadShipParameters(); err != nil {
		t.Fatal(err)
	}

	if len(ships) < 100 {
		t.Fatalf("expected a useful ship catalog, got %d ships", len(ships))
	}

	if ship, found = ships[4284397360]; !found ||
		ship.Species != "Cruiser" ||
		ship.Nation != "Germany" {
		t.Fatalf("expected Prinz Adalbert cruiser, got %#v, found=%v", ship, found)
	}

	if borders, err = bundle.LoadMapBorderParameters(); err != nil {
		t.Fatal(err)
	}
	if border, borderFound = borders[4289513392]; !borderFound || border.Width != 1400 || border.Height != 1400 {
		t.Fatalf("expected 14x14 map border, got %#v, found=%v", border, borderFound)
	}
}

// TestLoadProjectileParameters extracts real torpedo speed and range data.
func TestLoadProjectileParameters(t *testing.T) {
	var (
		bundle      *ResourceBundle
		err         error
		projectile  ProjectileParameters
		projectiles map[uint64]ProjectileParameters
		root        string = filepath.Join("testdata", "game", "15.6.0_12830008")
	)

	if _, err = os.Stat(root); os.IsNotExist(err) {
		t.Skip("imported 15.6 resource fixture is not present")
	}
	if bundle, err = OpenResourceBundle(root); err != nil {
		t.Fatal(err)
	}
	if projectiles, err = bundle.LoadProjectileParameters(); err != nil {
		t.Fatal(err)
	}
	projectile = projectiles[4279649072]
	if projectile.Species != "Torpedo" ||
		projectile.Speed != 65 ||
		projectile.MaxDistance != 267 {
		t.Fatalf("unexpected torpedo parameters: %#v", projectile)
	}
}
