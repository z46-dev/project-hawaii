package wowsreplay

import "testing"

// TestMergeMatchesUnionsPerspectiveTracks validates fog-of-war perspective merging.
func TestMergeMatchesUnionsPerspectiveTracks(t *testing.T) {
	var (
		first *Match = &Match{
			Metadata: Metadata{MapName: "spaces/test", DateTime: "now", TeamsCount: 2},
			ArenaID:  42,
			Entities: map[uint32]*MatchEntity{
				1: {ID: 1, Type: "Vehicle", TeamKnown: true, TeamID: 0, Positions: []MatchPosition{{Clock: 1}}},
			},
			PerspectiveCount: 1,
			PerspectiveTeams: []int32{0},
			Unknown:          MatchUnknownSummary{PacketTypes: make(map[string]int)},
		}
		second *Match = &Match{
			Metadata: Metadata{MapName: "spaces/test", DateTime: "now", TeamsCount: 2},
			ArenaID:  42,
			Entities: map[uint32]*MatchEntity{
				1: {ID: 1, Type: "Vehicle", TeamKnown: true, TeamID: 0, Positions: []MatchPosition{{Clock: 2}}},
				2: {ID: 2, Type: "Vehicle", TeamKnown: true, TeamID: 1, Positions: []MatchPosition{{Clock: 1}}},
			},
			PerspectiveCount: 1,
			PerspectiveTeams: []int32{1},
			Unknown:          MatchUnknownSummary{PacketTypes: make(map[string]int)},
		}
		merged *Match
		err    error
	)

	if merged, err = MergeMatches(first, second); err != nil {
		t.Fatal(err)
	}

	if merged.PerspectiveCount != 2 || merged.FogOfWarLimited {
		t.Fatalf("unexpected completeness state: %#v", merged)
	}

	if len(merged.Entities) != 2 || len(merged.Entities[1].Positions) != 2 {
		t.Fatalf("perspective tracks were not merged: %#v", merged.Entities)
	}
}

// TestMergeMatchesRejectsDifferentArena protects against unrelated replay fusion.
func TestMergeMatchesRejectsDifferentArena(t *testing.T) {
	var first *Match = &Match{
		Metadata: Metadata{MapName: "spaces/test", DateTime: "now"},
		ArenaID:  1,
		Entities: make(map[uint32]*MatchEntity),
		Unknown:  MatchUnknownSummary{PacketTypes: make(map[string]int)},
	}
	var second *Match = cloneMatch(first)
	var err error

	second.ArenaID = 2
	if _, err = MergeMatches(first, second); err == nil {
		t.Fatal("different arenas were accepted")
	}
}

// TestAppendTorpedoTracesUsesRangeLimitedLaunches reconstructs visible torpedo motion.
func TestAppendTorpedoTracesUsesRangeLimitedLaunches(t *testing.T) {
	var (
		match  *Match             = &Match{}
		method SemanticMethodData = torpedoMethod(
			9,
			17,
			Vector3{X: 10, Z: 20},
			Vector3{X: 10},
		)
	)

	match.appendTorpedoTraces(1, method)
	if len(match.Traces) != 1 {
		t.Fatalf("observable torpedo launch was not reconstructed: %#v", match.Traces)
	}
	if match.Traces[0].Duration != 24 || match.Traces[0].End.X != 250 {
		t.Fatalf("standalone torpedo fallback is not range-limited: %#v", match.Traces[0])
	}
	match.EnrichProjectileParameters(map[uint64]ProjectileParameters{
		5: {ID: 5, Species: "Torpedo", MaxDistance: 100},
	})
	if match.Traces[0].Duration != 10 ||
		match.Traces[0].Range != 100 ||
		match.Traces[0].End.X != 110 {
		t.Fatalf("GameParams torpedo range was not applied: %#v", match.Traces[0])
	}
}

// TestAppendPackedTracesMarksSecondaryShells preserves flight time and gun role.
func TestAppendPackedTracesMarksSecondaryShells(t *testing.T) {
	var (
		match  *Match             = &Match{}
		method SemanticMethodData = SemanticMethodData{
			Arguments: []SemanticArgument{{
				Value: []any{map[string]any{
					"ownerID":  uint32(4),
					"paramsID": uint32(8),
					"shots": []any{map[string]any{
						"gunBarrelID":    uint32(32768),
						"shotID":         uint32(3),
						"pos":            Vector3{X: 1},
						"tarPos":         Vector3{X: 20},
						"serverTimeLeft": float32(7),
					}},
				}},
			}},
		}
	)

	match.appendPackedTraces(2, method, "shell", "shots", "pos", "tarPos", "serverTimeLeft")
	if len(match.Traces) != 1 {
		t.Fatalf("shell trace was not reconstructed: %#v", match.Traces)
	}
	if !match.Traces[0].Secondary ||
		match.Traces[0].Duration != 7 ||
		match.Traces[0].End.X != 20 {
		t.Fatalf("secondary shell semantics were lost: %#v", match.Traces[0])
	}
}

// torpedoMethod creates one decoded torpedo launch for reconstruction tests.
func torpedoMethod(
	ownerID,
	shotID uint32,
	position,
	direction Vector3,
) (method SemanticMethodData) {
	method = SemanticMethodData{
		Arguments: []SemanticArgument{{
			Value: []any{map[string]any{
				"ownerID":  ownerID,
				"paramsID": uint32(5),
				"torpedoes": []any{map[string]any{
					"shotID": shotID,
					"pos":    position,
					"dir":    direction,
				}},
			}},
		}},
	}

	return
}
