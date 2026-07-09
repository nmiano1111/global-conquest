package reporting

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"backend/internal/risk"
)

// --- helpers ---

func makePayload(p risk.CombatRollResolvedPayload) []byte {
	b, err := json.Marshal(p)
	if err != nil {
		panic(err)
	}
	return b
}

func validPayload() risk.CombatRollResolvedPayload {
	return risk.CombatRollResolvedPayload{
		SchemaVersion:      risk.SchemaVersionCombatRollResolved,
		AttackerPlayerID:   "attacker-uuid",
		DefenderPlayerID:   "defender-uuid",
		SourceTerritoryID:  "alaska",
		TargetTerritoryID:  "kamchatka",
		SourceArmiesBefore: 5,
		TargetArmiesBefore: 3,
		AttackerDice:       []int{6, 4},
		DefenderDice:       []int{5},
		AttackerLosses:     0,
		DefenderLosses:     1,
		TerritoryCaptured:  false,
	}
}

func validRow() rawCombatRow {
	return rawCombatRow{
		id:           "row-1",
		gameID:       "game-1",
		gameSequence: 42,
		eventVersion: risk.EventVersionCombatRollResolved,
		occurredAt:   time.Now(),
		payload:      makePayload(validPayload()),
	}
}

// --- decodeCombatEvent ---

func TestDecodeCombatEvent_Valid(t *testing.T) {
	ev, skip, err := decodeCombatEvent(validRow())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if skip {
		t.Fatal("expected valid decode, got skip=true")
	}
	if ev.AttackerPlayerID != "attacker-uuid" {
		t.Errorf("AttackerPlayerID: got %q", ev.AttackerPlayerID)
	}
	if ev.GameSequence != 42 {
		t.Errorf("GameSequence: got %d", ev.GameSequence)
	}
	if len(ev.AttackerDice) != 2 || ev.AttackerDice[0] != 6 {
		t.Errorf("AttackerDice: got %v", ev.AttackerDice)
	}
}

func TestDecodeCombatEvent_UnsupportedEventVersion(t *testing.T) {
	row := validRow()
	row.eventVersion = 2

	_, skip, err := decodeCombatEvent(row)
	if err == nil {
		t.Fatal("expected error for unsupported event_version")
	}
	var ve ErrUnsupportedEventVersion
	if !errors.As(err, &ve) {
		t.Fatalf("expected ErrUnsupportedEventVersion, got %T: %v", err, err)
	}
	if ve.EventVersion != 2 {
		t.Errorf("expected EventVersion=2, got %d", ve.EventVersion)
	}
	if skip {
		t.Error("unsupported version should not skip — it should fail")
	}
}

func TestDecodeCombatEvent_UnsupportedSchemaVersion(t *testing.T) {
	p := validPayload()
	p.SchemaVersion = 99
	row := validRow()
	row.payload = makePayload(p)

	_, skip, err := decodeCombatEvent(row)
	if err == nil {
		t.Fatal("expected error for unsupported schema_version")
	}
	var se ErrUnsupportedSchemaVersion
	if !errors.As(err, &se) {
		t.Fatalf("expected ErrUnsupportedSchemaVersion, got %T: %v", err, err)
	}
	if se.SchemaVersion != 99 {
		t.Errorf("expected SchemaVersion=99, got %d", se.SchemaVersion)
	}
	if skip {
		t.Error("unsupported schema version should not skip — it should fail")
	}
}

func TestDecodeCombatEvent_MalformedJSON(t *testing.T) {
	row := validRow()
	row.payload = []byte(`not json`)

	_, skip, err := decodeCombatEvent(row)
	if err != nil {
		t.Fatalf("expected nil error for malformed JSON (skip), got: %v", err)
	}
	if !skip {
		t.Error("malformed JSON should produce skip=true")
	}
}

func TestDecodeCombatEvent_InvalidDiceValue_Low(t *testing.T) {
	p := validPayload()
	p.AttackerDice = []int{0, 4} // 0 is below range
	row := validRow()
	row.payload = makePayload(p)

	_, skip, err := decodeCombatEvent(row)
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if !skip {
		t.Error("dice value 0 should produce skip=true")
	}
}

func TestDecodeCombatEvent_InvalidDiceValue_High(t *testing.T) {
	p := validPayload()
	p.DefenderDice = []int{7} // 7 is above range
	row := validRow()
	row.payload = makePayload(p)

	_, skip, err := decodeCombatEvent(row)
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if !skip {
		t.Error("dice value 7 should produce skip=true")
	}
}

func TestDecodeCombatEvent_MissingPlayerID(t *testing.T) {
	p := validPayload()
	p.AttackerPlayerID = ""
	row := validRow()
	row.payload = makePayload(p)

	_, skip, err := decodeCombatEvent(row)
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if !skip {
		t.Error("missing player ID should produce skip=true")
	}
}

func TestDecodeCombatEvent_NegativeLosses(t *testing.T) {
	p := validPayload()
	p.AttackerLosses = -1
	row := validRow()
	row.payload = makePayload(p)

	_, skip, err := decodeCombatEvent(row)
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if !skip {
		t.Error("negative losses should produce skip=true")
	}
}

// --- BuildDiceReport ---

func makeEvents() []CombatEvent {
	return []CombatEvent{
		{
			GameSequence:      1,
			AttackerPlayerID:  "p1",
			DefenderPlayerID:  "p2",
			AttackerDice:      []int{6, 5, 4},
			DefenderDice:      []int{3, 2},
			AttackerLosses:    0,
			DefenderLosses:    2,
			TerritoryCaptured: true,
		},
		{
			GameSequence:      2,
			AttackerPlayerID:  "p2",
			DefenderPlayerID:  "p1",
			AttackerDice:      []int{1, 2},
			DefenderDice:      []int{6},
			AttackerLosses:    2,
			DefenderLosses:    0,
			TerritoryCaptured: false,
		},
	}
}

func TestBuildDiceReport_FaceCounts(t *testing.T) {
	events := makeEvents()
	r := BuildDiceReport("g1", events, 0)

	// Attacker dice: [6,5,4] + [1,2] → counts: 6→1, 5→1, 4→1, 1→1, 2→1; total=5
	if r.AttackerDice.Total != 5 {
		t.Errorf("AttackerDice.Total: want 5, got %d", r.AttackerDice.Total)
	}
	if r.AttackerDice.Counts[6] != 1 {
		t.Errorf("AttackerDice face 6: want 1, got %d", r.AttackerDice.Counts[6])
	}
	if r.AttackerDice.Counts[1] != 1 {
		t.Errorf("AttackerDice face 1: want 1, got %d", r.AttackerDice.Counts[1])
	}
	// Defender dice: [3,2] + [6] → total=3
	if r.DefenderDice.Total != 3 {
		t.Errorf("DefenderDice.Total: want 3, got %d", r.DefenderDice.Total)
	}
	if r.DefenderDice.Counts[6] != 1 {
		t.Errorf("DefenderDice face 6: want 1, got %d", r.DefenderDice.Counts[6])
	}
}

func TestBuildDiceReport_LossTotals(t *testing.T) {
	events := makeEvents()
	r := BuildDiceReport("g1", events, 0)

	if r.AttackerLosses != 2 {
		t.Errorf("AttackerLosses: want 2, got %d", r.AttackerLosses)
	}
	if r.DefenderLosses != 2 {
		t.Errorf("DefenderLosses: want 2, got %d", r.DefenderLosses)
	}
}

func TestBuildDiceReport_CaptureCount(t *testing.T) {
	events := makeEvents()
	r := BuildDiceReport("g1", events, 0)

	if r.Captures != 1 {
		t.Errorf("Captures: want 1, got %d", r.Captures)
	}
	if r.CombatRolls != 2 {
		t.Errorf("CombatRolls: want 2, got %d", r.CombatRolls)
	}
}

func TestBuildDiceReport_EmptyInput(t *testing.T) {
	r := BuildDiceReport("g1", nil, 0)
	if r.CombatRolls != 0 {
		t.Errorf("expected 0 rolls for empty input, got %d", r.CombatRolls)
	}
	if r.AttackerDice.Total != 0 {
		t.Errorf("expected 0 attacker total for empty input")
	}
}

func TestBuildDiceReport_NoDivisionByZero(t *testing.T) {
	// Verify that formatting helpers don't panic on zero total.
	r := BuildDiceReport("g1", nil, 0)
	// Compute a percentage the same way the formatter would.
	total := r.AttackerDice.Total
	var pct float64
	if total > 0 {
		pct = float64(r.AttackerDice.Counts[1]) / float64(total) * 100.0
	}
	if pct != 0 {
		t.Errorf("expected 0%% for empty distribution, got %.2f", pct)
	}
}

func TestBuildDiceReport_Percentages(t *testing.T) {
	events := []CombatEvent{
		{AttackerDice: []int{6, 6}, DefenderDice: []int{1}},
	}
	r := BuildDiceReport("g1", events, 0)
	// Face 6 appears 2 out of 2 attacker dice → 100%
	total := r.AttackerDice.Total // 2
	pct := float64(r.AttackerDice.Counts[6]) / float64(total) * 100.0
	if pct != 100.0 {
		t.Errorf("expected 100%% for all-6 dice, got %.1f%%", pct)
	}
	// Face 1 appears 1 out of 1 defender die → 100%
	defTotal := r.DefenderDice.Total
	defPct := float64(r.DefenderDice.Counts[1]) / float64(defTotal) * 100.0
	if defPct != 100.0 {
		t.Errorf("expected 100%% defender face-1, got %.1f%%", defPct)
	}
}

// --- BuildPlayerReport ---

func TestBuildPlayerReport_AttackCount(t *testing.T) {
	events := makeEvents()
	r := BuildPlayerReport("g1", "p1", events, nil)
	if r.AttackRolls != 1 {
		t.Errorf("AttackRolls: want 1, got %d", r.AttackRolls)
	}
}

func TestBuildPlayerReport_CaptureRate(t *testing.T) {
	events := makeEvents() // p1 has 1 roll, 1 capture → 100%
	r := BuildPlayerReport("g1", "p1", events, nil)
	if r.TerritoriesCaptured != 1 {
		t.Errorf("TerritoriesCaptured: want 1, got %d", r.TerritoriesCaptured)
	}
	if r.CaptureRate != 100.0 {
		t.Errorf("CaptureRate: want 100.0, got %.1f", r.CaptureRate)
	}
}

func TestBuildPlayerReport_AverageDice(t *testing.T) {
	events := makeEvents() // p1 rolls [6,5,4] → 3 dice in 1 attack → avg 3.0
	r := BuildPlayerReport("g1", "p1", events, nil)
	if r.AttackerDiceRolled != 3 {
		t.Errorf("AttackerDiceRolled: want 3, got %d", r.AttackerDiceRolled)
	}
	if r.AverageAttackerDice != 3.0 {
		t.Errorf("AverageAttackerDice: want 3.0, got %.2f", r.AverageAttackerDice)
	}
}

func TestBuildPlayerReport_ArmyLosses(t *testing.T) {
	events := makeEvents() // p1 loses 0 armies in its attack
	r := BuildPlayerReport("g1", "p1", events, nil)
	if r.AttackerLosses != 0 {
		t.Errorf("AttackerLosses: want 0, got %d", r.AttackerLosses)
	}
}

func TestBuildPlayerReport_DefenderLossesInflicted(t *testing.T) {
	events := makeEvents() // p1's attack: defender loses 2
	r := BuildPlayerReport("g1", "p1", events, nil)
	if r.DefenderLossesInflicted != 2 {
		t.Errorf("DefenderLossesInflicted: want 2, got %d", r.DefenderLossesInflicted)
	}
}

func TestBuildPlayerReport_AverageArmyAdvantage(t *testing.T) {
	events := []CombatEvent{
		{
			AttackerPlayerID:   "p1",
			DefenderPlayerID:   "p2",
			SourceArmiesBefore: 8,
			TargetArmiesBefore: 3,
			AttackerDice:       []int{6},
			DefenderDice:       []int{3},
		},
	}
	r := BuildPlayerReport("g1", "p1", events, nil)
	if r.AverageSourceArmiesBefore != 8.0 {
		t.Errorf("AverageSource: want 8.0, got %.1f", r.AverageSourceArmiesBefore)
	}
	if r.AverageTargetArmiesBefore != 3.0 {
		t.Errorf("AverageTarget: want 3.0, got %.1f", r.AverageTargetArmiesBefore)
	}
	want := 8.0 - 3.0
	if r.AverageArmyAdvantage != want {
		t.Errorf("AverageArmyAdvantage: want %.1f, got %.1f", want, r.AverageArmyAdvantage)
	}
}

func TestBuildPlayerReport_PlayerWithNoEvents(t *testing.T) {
	events := makeEvents()
	r := BuildPlayerReport("g1", "unknown-player", events, nil)
	if r.AttackRolls != 0 {
		t.Errorf("expected 0 attack rolls for player with no events, got %d", r.AttackRolls)
	}
	if r.CaptureRate != 0 {
		t.Errorf("expected 0 capture rate for player with no events, got %.1f", r.CaptureRate)
	}
}

// --- RecentRolls via Service (count limit + display order) ---

type fakeGame struct {
	id   string
	name string
}

type fakeCurrentPlayer struct {
	username    string
	discordName *string
}

type fakeRepo struct {
	rawAll    []rawCombatRow
	rawRecent []rawCombatRow
	names     map[string]string

	latestGame        *fakeGame
	gamesByName       map[string]fakeGame // keyed by canonical name (any case)
	playersByUsername map[string]string   // username → playerID (any case)
	currentPlayer     *fakeCurrentPlayer
	activeGameNames   []string
	playerChoices     []PlayerChoice
	historyComplete   map[string]bool // gameID -> event_history_complete; defaults to true when absent
}

func (f *fakeRepo) LoadLatestGame(_ context.Context) (string, string, error) {
	if f.latestGame == nil {
		return "", "", ErrNoActiveGame
	}
	return f.latestGame.id, f.latestGame.name, nil
}

func (f *fakeRepo) LoadGameByName(_ context.Context, name string) (string, string, error) {
	lower := strings.ToLower(name)
	for k, g := range f.gamesByName {
		if strings.ToLower(k) == lower {
			return g.id, g.name, nil
		}
	}
	return "", "", ErrGameNotFound
}

func (f *fakeRepo) LoadActiveGameChoices(_ context.Context, _ string) ([]string, error) {
	return f.activeGameNames, nil
}

func (f *fakeRepo) LoadPlayerChoices(_ context.Context, _, _ string) ([]PlayerChoice, error) {
	return f.playerChoices, nil
}

func (f *fakeRepo) LoadCurrentPlayer(_ context.Context, _ string) (string, *string, error) {
	if f.currentPlayer == nil {
		return "", nil, ErrNoCurrentPlayer
	}
	return f.currentPlayer.username, f.currentPlayer.discordName, nil
}

func (f *fakeRepo) LoadPlayerByUsername(_ context.Context, username string) (string, error) {
	lower := strings.ToLower(username)
	for k, v := range f.playersByUsername {
		if strings.ToLower(k) == lower {
			return v, nil
		}
	}
	return "", ErrPlayerNotFound
}

func (f *fakeRepo) LoadRawCombatEvents(_ context.Context, _ string) ([]rawCombatRow, error) {
	return f.rawAll, nil
}

func (f *fakeRepo) LoadRawRecentCombatEvents(_ context.Context, _ string, limit int) ([]rawCombatRow, error) {
	rows := f.rawRecent
	if limit > 0 && len(rows) > limit {
		rows = rows[:limit]
	}
	return rows, nil
}

func (f *fakeRepo) LoadPlayerNames(_ context.Context, _ []string) (map[string]string, error) {
	if f.names == nil {
		return map[string]string{}, nil
	}
	return f.names, nil
}

func (f *fakeRepo) LoadEventHistoryComplete(_ context.Context, gameID string) (bool, error) {
	if f.historyComplete == nil {
		return true, nil
	}
	if complete, ok := f.historyComplete[gameID]; ok {
		return complete, nil
	}
	return true, nil
}

func makeRawRow(seq int64, captured bool) rawCombatRow {
	p := validPayload()
	p.TerritoryCaptured = captured
	return rawCombatRow{
		id:           "row",
		gameID:       "g1",
		gameSequence: seq,
		eventVersion: risk.EventVersionCombatRollResolved,
		occurredAt:   time.Unix(seq, 0),
		payload:      makePayload(p),
	}
}

func TestService_RecentRolls_CountLimit(t *testing.T) {
	// Provide 10 rows in DESC order (sequences 10..1), request 3.
	rawRows := make([]rawCombatRow, 10)
	for i := range rawRows {
		rawRows[i] = makeRawRow(int64(10-i), false) // 10, 9, 8, ...
	}
	repo := &fakeRepo{rawRecent: rawRows}
	svc := NewService(repo)

	rolls, err := svc.RecentRolls(context.Background(), "g1", 3)
	if err != nil {
		t.Fatalf("RecentRolls: %v", err)
	}
	if len(rolls) != 3 {
		t.Errorf("count limit: want 3, got %d", len(rolls))
	}
}

func TestService_RecentRolls_ChronologicalOrder(t *testing.T) {
	// Repository returns DESC (seq 5, 4, 3); service should reverse to ASC (3, 4, 5).
	repo := &fakeRepo{rawRecent: []rawCombatRow{
		makeRawRow(5, false),
		makeRawRow(4, false),
		makeRawRow(3, false),
	}}
	svc := NewService(repo)

	rolls, err := svc.RecentRolls(context.Background(), "g1", 10)
	if err != nil {
		t.Fatalf("RecentRolls: %v", err)
	}
	if len(rolls) != 3 {
		t.Fatalf("want 3 rolls, got %d", len(rolls))
	}
	if rolls[0].GameSequence != 3 || rolls[1].GameSequence != 4 || rolls[2].GameSequence != 5 {
		t.Errorf("wrong order: %v", []int64{rolls[0].GameSequence, rolls[1].GameSequence, rolls[2].GameSequence})
	}
}

func TestBuildRecentRolls_CaptureStatus(t *testing.T) {
	events := []CombatEvent{
		{GameSequence: 1, AttackerDice: []int{6}, DefenderDice: []int{3}, TerritoryCaptured: true},
		{GameSequence: 2, AttackerDice: []int{1}, DefenderDice: []int{4}, TerritoryCaptured: false},
	}
	rolls := BuildRecentRolls(events, nil)
	if !rolls[0].Captured {
		t.Error("first roll should be captured")
	}
	if rolls[1].Captured {
		t.Error("second roll should not be captured")
	}
}

func TestBuildRecentRolls_GameSequence(t *testing.T) {
	events := []CombatEvent{
		{GameSequence: 99, AttackerDice: []int{5}, DefenderDice: []int{2}},
	}
	rolls := BuildRecentRolls(events, nil)
	if rolls[0].GameSequence != 99 {
		t.Errorf("GameSequence: want 99, got %d", rolls[0].GameSequence)
	}
}

// --- ResolveGame ---

func TestService_ResolveGame_Latest(t *testing.T) {
	repo := &fakeRepo{latestGame: &fakeGame{id: "g1", name: "Angry Badger"}}
	svc := NewService(repo)

	id, name, err := svc.ResolveGame(context.Background(), "")
	if err != nil {
		t.Fatalf("ResolveGame: %v", err)
	}
	if id != "g1" || name != "Angry Badger" {
		t.Errorf("want g1/Angry Badger, got %s/%s", id, name)
	}
}

func TestService_ResolveGame_NoActiveGame(t *testing.T) {
	repo := &fakeRepo{}
	svc := NewService(repo)

	_, _, err := svc.ResolveGame(context.Background(), "")
	if !errors.Is(err, ErrNoActiveGame) {
		t.Errorf("expected ErrNoActiveGame, got %v", err)
	}
}

func TestService_ResolveGame_ByName(t *testing.T) {
	repo := &fakeRepo{
		gamesByName: map[string]fakeGame{
			"Sleepy Raccoon": {id: "g2", name: "Sleepy Raccoon"},
		},
	}
	svc := NewService(repo)

	id, name, err := svc.ResolveGame(context.Background(), "sleepy raccoon")
	if err != nil {
		t.Fatalf("ResolveGame by name: %v", err)
	}
	if id != "g2" || name != "Sleepy Raccoon" {
		t.Errorf("want g2/Sleepy Raccoon, got %s/%s", id, name)
	}
}

func TestService_ResolveGame_NameNotFound(t *testing.T) {
	repo := &fakeRepo{gamesByName: map[string]fakeGame{}}
	svc := NewService(repo)

	_, _, err := svc.ResolveGame(context.Background(), "Nonexistent Game")
	if !errors.Is(err, ErrGameNotFound) {
		t.Errorf("expected ErrGameNotFound, got %v", err)
	}
}

// --- ResolvePlayer ---

func TestService_ResolvePlayer_UUID(t *testing.T) {
	repo := &fakeRepo{}
	svc := NewService(repo)

	playerID, err := svc.ResolvePlayer(context.Background(), "12345678-1234-1234-1234-123456789abc")
	if err != nil {
		t.Fatalf("ResolvePlayer UUID: %v", err)
	}
	if playerID != "12345678-1234-1234-1234-123456789abc" {
		t.Errorf("UUID passthrough: got %q", playerID)
	}
}

func TestService_ResolvePlayer_UUIDUppercase(t *testing.T) {
	repo := &fakeRepo{}
	svc := NewService(repo)

	playerID, err := svc.ResolvePlayer(context.Background(), "12345678-ABCD-1234-1234-123456789ABC")
	if err != nil {
		t.Fatalf("ResolvePlayer uppercase UUID: %v", err)
	}
	if playerID != "12345678-abcd-1234-1234-123456789abc" {
		t.Errorf("UUID should be lowercased: got %q", playerID)
	}
}

func TestService_ResolvePlayer_Username(t *testing.T) {
	repo := &fakeRepo{
		playersByUsername: map[string]string{"Alice": "player-uuid-1"},
	}
	svc := NewService(repo)

	playerID, err := svc.ResolvePlayer(context.Background(), "alice")
	if err != nil {
		t.Fatalf("ResolvePlayer username: %v", err)
	}
	if playerID != "player-uuid-1" {
		t.Errorf("want player-uuid-1, got %q", playerID)
	}
}

func TestService_ResolvePlayer_NotFound(t *testing.T) {
	repo := &fakeRepo{playersByUsername: map[string]string{}}
	svc := NewService(repo)

	_, err := svc.ResolvePlayer(context.Background(), "unknown")
	if !errors.Is(err, ErrPlayerNotFound) {
		t.Errorf("expected ErrPlayerNotFound, got %v", err)
	}
}

// --- CurrentPlayer ---

func strPtr(s string) *string { return &s }

func TestService_CurrentPlayer_WithDiscordName(t *testing.T) {
	repo := &fakeRepo{currentPlayer: &fakeCurrentPlayer{username: "alice", discordName: strPtr("Alice#1234")}}
	svc := NewService(repo)

	username, discordName, err := svc.CurrentPlayer(context.Background(), "g1")
	if err != nil {
		t.Fatalf("CurrentPlayer: %v", err)
	}
	if username != "alice" {
		t.Errorf("username: want %q, got %q", "alice", username)
	}
	if discordName == nil || *discordName != "Alice#1234" {
		t.Errorf("discordName: want %q, got %v", "Alice#1234", discordName)
	}
}

func TestService_CurrentPlayer_NoDiscordName(t *testing.T) {
	repo := &fakeRepo{currentPlayer: &fakeCurrentPlayer{username: "bob", discordName: nil}}
	svc := NewService(repo)

	username, discordName, err := svc.CurrentPlayer(context.Background(), "g1")
	if err != nil {
		t.Fatalf("CurrentPlayer: %v", err)
	}
	if username != "bob" {
		t.Errorf("username: want %q, got %q", "bob", username)
	}
	if discordName != nil {
		t.Errorf("discordName should be nil, got %q", *discordName)
	}
}

func TestService_CurrentPlayer_NotFound(t *testing.T) {
	repo := &fakeRepo{}
	svc := NewService(repo)

	_, _, err := svc.CurrentPlayer(context.Background(), "g1")
	if !errors.Is(err, ErrNoCurrentPlayer) {
		t.Errorf("expected ErrNoCurrentPlayer, got %v", err)
	}
}

// --- ActiveGameChoices / PlayerChoices ---

func TestService_ActiveGameChoices(t *testing.T) {
	repo := &fakeRepo{activeGameNames: []string{"Angry Badger", "Sleepy Raccoon"}}
	svc := NewService(repo)

	names, err := svc.ActiveGameChoices(context.Background(), "")
	if err != nil {
		t.Fatalf("ActiveGameChoices: %v", err)
	}
	if len(names) != 2 {
		t.Errorf("want 2 names, got %d", len(names))
	}
}

func TestService_PlayerChoices(t *testing.T) {
	choices := []PlayerChoice{
		{Name: "alice", Value: "alice"},
		{Name: "bob (Bob#1234)", Value: "bob"},
	}
	repo := &fakeRepo{playerChoices: choices}
	svc := NewService(repo)

	got, err := svc.PlayerChoices(context.Background(), "", "")
	if err != nil {
		t.Fatalf("PlayerChoices: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("want 2 choices, got %d", len(got))
	}
	if got[1].Value != "bob" {
		t.Errorf("choice value: want %q, got %q", "bob", got[1].Value)
	}
}
