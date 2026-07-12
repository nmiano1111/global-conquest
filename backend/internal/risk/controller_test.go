package risk

import (
	"encoding/json"
	"testing"
)

// TestPlayerStateBackwardCompatibleDeserialization verifies that games
// serialized before Controller/Strategy existed still decode, defaulting to
// human with no strategy assigned.
func TestPlayerStateBackwardCompatibleDeserialization(t *testing.T) {
	const legacyJSON = `{"id":"p1","cards":[],"eliminated":false}`
	var p PlayerState
	if err := json.Unmarshal([]byte(legacyJSON), &p); err != nil {
		t.Fatalf("unmarshal legacy player state: %v", err)
	}
	if p.IsBot() {
		t.Fatalf("expected legacy player without controller field to default to human")
	}
	if p.Controller != "" {
		t.Fatalf("expected empty Controller, got %q", p.Controller)
	}
	if p.Strategy != "" {
		t.Fatalf("expected empty Strategy, got %q", p.Strategy)
	}
}

// TestLegacyGameStateDeserialization checks a whole Game blob (as it would
// be stored in games.state) predating the controller fields still decodes
// and every player reports as human.
func TestLegacyGameStateDeserialization(t *testing.T) {
	const legacyGame = `{
		"board": {"continents": {}, "adjacent": {}, "order": []},
		"players": [{"id":"p1","cards":[],"eliminated":false},{"id":"p2","cards":[],"eliminated":false}],
		"territories": {},
		"current_player": 0,
		"phase": "reinforce",
		"turn_number": 1
	}`
	var g Game
	if err := json.Unmarshal([]byte(legacyGame), &g); err != nil {
		t.Fatalf("unmarshal legacy game: %v", err)
	}
	for _, p := range g.Players {
		if p.IsBot() {
			t.Fatalf("player %s: expected human default, got bot", p.ID)
		}
	}
}

// TestBotPlayerRoundTrips ensures a bot-controlled player retains its
// Controller and Strategy through a marshal/unmarshal cycle, and that human
// players (zero-value Controller) are unaffected by the new fields.
func TestBotPlayerRoundTrips(t *testing.T) {
	g := mustGame(t)
	g.Players[0].Controller = ControllerBot
	g.Players[0].Strategy = "basic-v1"

	raw, err := json.Marshal(g)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out Game
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !out.Players[0].IsBot() {
		t.Fatalf("expected player 0 to round-trip as bot")
	}
	if out.Players[0].Strategy != "basic-v1" {
		t.Fatalf("expected strategy basic-v1, got %q", out.Players[0].Strategy)
	}
	for i := 1; i < len(out.Players); i++ {
		if out.Players[i].IsBot() {
			t.Fatalf("player %d: expected human, got bot", i)
		}
	}
}
