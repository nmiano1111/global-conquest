package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"backend/internal/db"
	"backend/internal/risk"
	"backend/internal/store"
	"github.com/jackc/pgx/v5"
)

type gameDB interface {
	Queryer() db.Querier
	WithTxQ(ctx context.Context, fn func(q db.Querier) error) error
}

type GamesService struct {
	db        gameDB
	games     store.GamesStore
	gameEvent gameEventStore
}

var (
	ErrGameNotFound        = errors.New("game not found")
	ErrInvalidGameInput    = errors.New("invalid game input")
	ErrUnknownPlayerIDs    = errors.New("one or more player_ids do not exist")
	ErrGameNotJoinable     = errors.New("game is not joinable")
	ErrGameAlreadyJoined   = errors.New("player already joined this game")
	ErrGamePlayerCountFull = errors.New("game is already full")
	ErrGameForbidden       = errors.New("game access forbidden")
	ErrInvalidGameAction   = errors.New("invalid game action")
)

func NewGamesService(db gameDB, games store.GamesStore) *GamesService {
	return &GamesService{db: db, games: games}
}

type gameEventStore interface {
	SaveGameEvent(ctx context.Context, q db.Querier, gameID, actorUserID, eventType, body string) (store.GameEvent, error)
	ListGameEvents(ctx context.Context, q db.Querier, gameID string, limit int) ([]store.GameEvent, error)
}

func (s *GamesService) SetGameEventStore(gameEvent gameEventStore) {
	s.gameEvent = gameEvent
}

type lobbyState struct {
	PlayerCount int      `json:"player_count"`
	PlayerIDs   []string `json:"player_ids"`
}

type GameBootstrapPlayer struct {
	UserID     string `json:"user_id"`
	UserName   string `json:"user_name"`
	Color      string `json:"color"`
	CardCount  int    `json:"card_count"`
	Eliminated bool   `json:"eliminated"`
}

type GameBootstrap struct {
	ID                    string                 `json:"id"`
	OwnerUserID           string                 `json:"owner_user_id"`
	Status                string                 `json:"status"`
	Phase                 string                 `json:"phase"`
	CurrentPlayer         int                    `json:"current_player"`
	PendingReinforcements int                    `json:"pending_reinforcements"`
	Occupy                *GameOccupyRequirement `json:"occupy,omitempty"`
	Players               []GameBootstrapPlayer  `json:"players"`
	Territories           json.RawMessage        `json:"territories"`
	Events                []GameEventEntry       `json:"events"`
	CreatedAt             time.Time              `json:"created_at"`
	UpdatedAt             time.Time              `json:"updated_at"`
}

type GameActionInput struct {
	GameID       string
	PlayerUserID string
	Action       string
	Territory    string
	From         string
	To           string
	Armies       int
	AttackerDice int
	DefenderDice int
}

type GameActionPlayer struct {
	UserID     string `json:"user_id"`
	CardCount  int    `json:"card_count"`
	Eliminated bool   `json:"eliminated"`
}

type GameActionUpdate struct {
	GameID                string                 `json:"game_id"`
	Action                string                 `json:"action"`
	ActorUserID           string                 `json:"actor_user_id"`
	Phase                 string                 `json:"phase"`
	CurrentPlayer         int                    `json:"current_player"`
	PendingReinforcements int                    `json:"pending_reinforcements"`
	Occupy                *GameOccupyRequirement `json:"occupy,omitempty"`
	Players               []GameActionPlayer     `json:"players"`
	Territories           json.RawMessage        `json:"territories"`
	Result                any                    `json:"result,omitempty"`
	Event                 *GameEventEntry        `json:"event,omitempty"`
}

type GameEventEntry struct {
	ID          string    `json:"id"`
	GameID      string    `json:"game_id"`
	ActorUserID string    `json:"actor_user_id,omitempty"`
	EventType   string    `json:"event_type"`
	Body        string    `json:"body"`
	CreatedAt   time.Time `json:"created_at"`
}

type GameOccupyRequirement struct {
	From    string `json:"from"`
	To      string `json:"to"`
	MinMove int    `json:"min_move"`
	MaxMove int    `json:"max_move"`
}

func (s *GamesService) CreateClassicGame(ctx context.Context, ownerUserID string, playerCount int) (store.Game, error) {
	if ownerUserID == "" {
		return store.Game{}, ErrInvalidGameInput
	}
	if playerCount < 3 || playerCount > 6 {
		return store.Game{}, ErrInvalidGameInput
	}

	var existingOwner int
	if err := s.db.Queryer().QueryRow(
		ctx,
		`SELECT count(*) FROM users WHERE id::text = $1`,
		ownerUserID,
	).Scan(&existingOwner); err != nil {
		return store.Game{}, err
	}
	if existingOwner != 1 {
		return store.Game{}, ErrUnknownPlayerIDs
	}

	state, err := json.Marshal(lobbyState{
		PlayerCount: playerCount,
		PlayerIDs:   []string{ownerUserID},
	})
	if err != nil {
		return store.Game{}, err
	}

	return s.games.Create(ctx, s.db.Queryer(), store.NewGame{
		OwnerUserID: ownerUserID,
		Status:      "lobby",
		State:       state,
	})
}

func (s *GamesService) JoinClassicGame(ctx context.Context, gameID, playerID string) (store.Game, error) {
	if gameID == "" || playerID == "" {
		return store.Game{}, ErrInvalidGameInput
	}

	var out store.Game
	err := s.db.WithTxQ(ctx, func(q db.Querier) error {
		g, err := s.games.GetByIDForUpdate(ctx, q, gameID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrGameNotFound
			}
			return err
		}
		if g.Status != "lobby" {
			return ErrGameNotJoinable
		}

		lobby, err := decodeLobbyState(g.State)
		if err != nil {
			return err
		}

		for _, id := range lobby.PlayerIDs {
			if id == playerID {
				out = g
				return nil
			}
		}

		if len(lobby.PlayerIDs) >= lobby.PlayerCount {
			return ErrGamePlayerCountFull
		}

		lobby.PlayerIDs = append(lobby.PlayerIDs, playerID)
		names, err := s.userNamesByIDsQ(ctx, q, lobby.PlayerIDs)
		if err != nil {
			return err
		}
		nextStatus := "lobby"
		var nextState []byte
		if len(lobby.PlayerIDs) == lobby.PlayerCount {
			engine, err := risk.NewClassicAutoStartGame(lobby.PlayerIDs, nil)
			if err != nil {
				return err
			}
			nextStatus = "in_progress"
			nextState, err = json.Marshal(engine)
			if err != nil {
				return err
			}
			if s.gameEvent != nil {
				joinBody := fmt.Sprintf(
					"%s joined the game lobby (%d/%d players).",
					displayName(names, playerID),
					len(lobby.PlayerIDs),
					lobby.PlayerCount,
				)
				if _, err := s.gameEvent.SaveGameEvent(ctx, q, g.ID, playerID, "player_joined", joinBody); err != nil {
					return err
				}
				startBody := fmt.Sprintf("All players joined. The game has started. %s goes first.", displayName(names, engine.Players[engine.CurrentPlayer].ID))
				if _, err := s.gameEvent.SaveGameEvent(ctx, q, g.ID, playerID, "game_started", startBody); err != nil {
					return err
				}
			}
		} else {
			nextState, err = json.Marshal(lobby)
			if err != nil {
				return err
			}
			if s.gameEvent != nil {
				joinBody := fmt.Sprintf(
					"%s joined the game lobby (%d/%d players).",
					displayName(names, playerID),
					len(lobby.PlayerIDs),
					lobby.PlayerCount,
				)
				if _, err := s.gameEvent.SaveGameEvent(ctx, q, g.ID, playerID, "player_joined", joinBody); err != nil {
					return err
				}
			}
		}

		out, err = s.games.UpdateState(ctx, q, store.UpdateGameState{
			GameID: g.ID,
			Status: nextStatus,
			State:  nextState,
		})
		return err
	})
	return out, err
}

func (s *GamesService) GetGame(ctx context.Context, gameID string) (store.Game, error) {
	g, err := s.games.GetByID(ctx, s.db.Queryer(), gameID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return store.Game{}, ErrGameNotFound
		}
		return store.Game{}, err
	}
	return g, nil
}

func (s *GamesService) ListGames(ctx context.Context, ownerUserID, status string, limit, offset int) ([]store.Game, error) {
	if limit < 0 || offset < 0 {
		return nil, ErrInvalidGameInput
	}
	return s.games.List(ctx, s.db.Queryer(), store.GameListFilter{
		OwnerUserID: ownerUserID,
		Status:      status,
		Limit:       limit,
		Offset:      offset,
	})
}

func (s *GamesService) UpdateGameState(ctx context.Context, gameID, status string, state json.RawMessage) (store.Game, error) {
	if gameID == "" || status == "" || len(state) == 0 {
		return store.Game{}, ErrInvalidGameInput
	}
	g, err := s.games.UpdateState(ctx, s.db.Queryer(), store.UpdateGameState{
		GameID: gameID,
		Status: status,
		State:  state,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return store.Game{}, ErrGameNotFound
		}
		return store.Game{}, err
	}
	return g, nil
}

func (s *GamesService) ApplyGameAction(ctx context.Context, in GameActionInput) (GameActionUpdate, error) {
	if in.GameID == "" || in.PlayerUserID == "" || in.Action == "" {
		return GameActionUpdate{}, ErrInvalidGameInput
	}

	var out GameActionUpdate
	err := s.db.WithTxQ(ctx, func(q db.Querier) error {
		g, err := s.games.GetByIDForUpdate(ctx, q, in.GameID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrGameNotFound
			}
			return err
		}
		if g.Status != "in_progress" {
			return ErrGameNotJoinable
		}

		var engine risk.Game
		if err := json.Unmarshal(g.State, &engine); err != nil {
			return ErrInvalidGameInput
		}
		playerIDs := make([]string, 0, len(engine.Players))
		for _, p := range engine.Players {
			playerIDs = append(playerIDs, p.ID)
		}
		if !containsID(playerIDs, in.PlayerUserID) {
			return ErrGameForbidden
		}
		names, err := s.userNamesByIDsQ(ctx, q, playerIDs)
		if err != nil {
			return err
		}
		prevOccupy := occupyRequirement(engine.Occupy)

		var result any
		var eventType, eventBody string
		switch in.Action {
		case "place_reinforcement":
			if in.Territory == "" || in.Armies <= 0 {
				return ErrInvalidGameAction
			}
			if err := engine.PlaceReinforcement(in.PlayerUserID, risk.Territory(in.Territory), in.Armies); err != nil {
				return err
			}
			eventType = "reinforcement_placed"
			eventBody = fmt.Sprintf("%s placed %d %s on %s.", displayName(names, in.PlayerUserID), in.Armies, pluralize("army", in.Armies), in.Territory)
		case "attack":
			if in.From == "" || in.To == "" || in.AttackerDice <= 0 {
				return ErrInvalidGameAction
			}
			src, ok := engine.Territories[risk.Territory(in.From)]
			if !ok {
				return ErrInvalidGameAction
			}
			dst, ok := engine.Territories[risk.Territory(in.To)]
			if !ok {
				return ErrInvalidGameAction
			}
			defenderUserID := ""
			if dst.Owner >= 0 && dst.Owner < len(engine.Players) {
				defenderUserID = engine.Players[dst.Owner].ID
			}
			maxAttackerDice := min(3, src.Armies-1)
			if maxAttackerDice < 1 {
				return ErrInvalidGameAction
			}
			attackerDice := min(max(1, in.AttackerDice), maxAttackerDice)
			defenderDice := min(2, dst.Armies)
			if defenderDice < 1 {
				return ErrInvalidGameAction
			}
			ar, err := engine.Attack(
				in.PlayerUserID,
				risk.Territory(in.From),
				risk.Territory(in.To),
				attackerDice,
				defenderDice,
			)
			if err != nil {
				return err
			}
			result = ar
			eventType = "attack_resolved"
			eventBody = fmt.Sprintf(
				"%s attacked %s from %s. Dice: attacker [%s], defender [%s]. Losses: %s %d, %s %d.",
				displayName(names, in.PlayerUserID),
				in.To,
				in.From,
				joinDice(ar.AttackerRolls),
				joinDice(ar.DefenderRolls),
				displayName(names, in.PlayerUserID),
				ar.AttackerLoss,
				displayName(names, defenderUserID),
				ar.DefenderLoss,
			)
			if ar.Conquered {
				eventBody += fmt.Sprintf(" %s conquered %s.", displayName(names, in.PlayerUserID), in.To)
			}
			if ar.Eliminated != "" {
				eventBody += fmt.Sprintf(" %s was eliminated.", displayName(names, ar.Eliminated))
			}
		case "occupy":
			if in.Armies <= 0 {
				return ErrInvalidGameAction
			}
			if err := engine.OccupyTerritory(in.PlayerUserID, in.Armies); err != nil {
				return err
			}
			from := in.From
			to := in.To
			if prevOccupy != nil {
				from = prevOccupy.From
				to = prevOccupy.To
			}
			eventType = "territory_occupied"
			eventBody = fmt.Sprintf("%s moved %d %s from %s to %s.", displayName(names, in.PlayerUserID), in.Armies, pluralize("army", in.Armies), from, to)
		case "end_attack":
			if err := engine.EndAttackPhase(in.PlayerUserID); err != nil {
				return err
			}
			eventType = "attack_phase_ended"
			eventBody = fmt.Sprintf("%s ended the attack phase.", displayName(names, in.PlayerUserID))
		case "fortify":
			if in.From == "" || in.To == "" || in.Armies <= 0 {
				return ErrInvalidGameAction
			}
			if err := engine.Fortify(in.PlayerUserID, risk.Territory(in.From), risk.Territory(in.To), in.Armies); err != nil {
				return err
			}
			eventType = "fortified"
			eventBody = fmt.Sprintf("%s fortified %s from %s with %d %s.", displayName(names, in.PlayerUserID), in.To, in.From, in.Armies, pluralize("army", in.Armies))
		case "end_turn":
			if err := engine.EndTurn(in.PlayerUserID); err != nil {
				return err
			}
			nextPlayer := ""
			if engine.CurrentPlayer >= 0 && engine.CurrentPlayer < len(engine.Players) {
				nextPlayer = engine.Players[engine.CurrentPlayer].ID
			}
			eventType = "turn_ended"
			eventBody = fmt.Sprintf("%s ended their turn. %s is up next.", displayName(names, in.PlayerUserID), displayName(names, nextPlayer))
		default:
			return ErrInvalidGameAction
		}

		nextState, err := json.Marshal(engine)
		if err != nil {
			return err
		}
		if _, err := s.games.UpdateState(ctx, q, store.UpdateGameState{
			GameID: g.ID,
			Status: "in_progress",
			State:  nextState,
		}); err != nil {
			return err
		}

		territories, err := json.Marshal(engine.Territories)
		if err != nil {
			return err
		}
		players := make([]GameActionPlayer, 0, len(engine.Players))
		for _, p := range engine.Players {
			players = append(players, GameActionPlayer{
				UserID:     p.ID,
				CardCount:  len(p.Cards),
				Eliminated: p.Eliminated,
			})
		}
		out = GameActionUpdate{
			GameID:                g.ID,
			Action:                in.Action,
			ActorUserID:           in.PlayerUserID,
			Phase:                 string(engine.Phase),
			CurrentPlayer:         engine.CurrentPlayer,
			PendingReinforcements: engine.PendingReinforcements,
			Occupy:                occupyRequirement(engine.Occupy),
			Players:               players,
			Territories:           territories,
			Result:                result,
		}
		if s.gameEvent != nil && strings.TrimSpace(eventBody) != "" {
			saved, err := s.gameEvent.SaveGameEvent(ctx, q, g.ID, in.PlayerUserID, eventType, eventBody)
			if err != nil {
				return err
			}
			out.Event = &GameEventEntry{
				ID:          saved.ID,
				GameID:      saved.GameID,
				ActorUserID: saved.ActorUserID,
				EventType:   saved.EventType,
				Body:        saved.Body,
				CreatedAt:   saved.CreatedAt,
			}
		}
		return nil
	})
	if err != nil {
		return GameActionUpdate{}, mapGameActionErr(err)
	}
	return out, nil
}

func (s *GamesService) GetGameBootstrap(ctx context.Context, gameID, requesterUserID string) (GameBootstrap, error) {
	if gameID == "" || requesterUserID == "" {
		return GameBootstrap{}, ErrInvalidGameInput
	}
	g, err := s.GetGame(ctx, gameID)
	if err != nil {
		return GameBootstrap{}, err
	}

	out := GameBootstrap{
		ID:          g.ID,
		OwnerUserID: g.OwnerUserID,
		Status:      g.Status,
		CreatedAt:   g.CreatedAt,
		UpdatedAt:   g.UpdatedAt,
	}

	switch g.Status {
	case "lobby":
		lobby, err := decodeLobbyState(g.State)
		if err != nil {
			return GameBootstrap{}, err
		}
		if !containsID(lobby.PlayerIDs, requesterUserID) {
			return GameBootstrap{}, ErrGameForbidden
		}
		names, err := s.userNamesByIDs(ctx, lobby.PlayerIDs)
		if err != nil {
			return GameBootstrap{}, err
		}
		out.Phase = "lobby"
		out.CurrentPlayer = -1
		out.PendingReinforcements = 0
		out.Occupy = nil
		out.Players = make([]GameBootstrapPlayer, 0, len(lobby.PlayerIDs))
		for _, id := range lobby.PlayerIDs {
			name := names[id]
			if name == "" {
				name = id
			}
			out.Players = append(out.Players, GameBootstrapPlayer{
				UserID:     id,
				UserName:   name,
				Color:      bootstrapColor(len(out.Players)),
				CardCount:  0,
				Eliminated: false,
			})
		}
		out.Territories = json.RawMessage(`{}`)
		if s.gameEvent != nil {
			events, err := s.gameEvent.ListGameEvents(ctx, s.db.Queryer(), g.ID, 250)
			if err != nil {
				return GameBootstrap{}, err
			}
			out.Events = make([]GameEventEntry, 0, len(events))
			for _, ev := range events {
				out.Events = append(out.Events, GameEventEntry{
					ID:          ev.ID,
					GameID:      ev.GameID,
					ActorUserID: ev.ActorUserID,
					EventType:   ev.EventType,
					Body:        ev.Body,
					CreatedAt:   ev.CreatedAt,
				})
			}
		}
		return out, nil

	case "in_progress":
		var engine risk.Game
		if err := json.Unmarshal(g.State, &engine); err != nil {
			return GameBootstrap{}, ErrInvalidGameInput
		}
		if isLegacyUninitializedSetup(engine) {
			ids := make([]string, 0, len(engine.Players))
			for _, p := range engine.Players {
				ids = append(ids, p.ID)
			}
			auto, err := risk.NewClassicAutoStartGame(ids, nil)
			if err != nil {
				return GameBootstrap{}, err
			}
			nextState, err := json.Marshal(auto)
			if err != nil {
				return GameBootstrap{}, err
			}
			updated, err := s.games.UpdateState(ctx, s.db.Queryer(), store.UpdateGameState{
				GameID: g.ID,
				Status: "in_progress",
				State:  nextState,
			})
			if err != nil {
				return GameBootstrap{}, err
			}
			g = updated
			engine = *auto
		}
		ids := make([]string, 0, len(engine.Players))
		for _, p := range engine.Players {
			ids = append(ids, p.ID)
		}
		if !containsID(ids, requesterUserID) {
			return GameBootstrap{}, ErrGameForbidden
		}
		names, err := s.userNamesByIDs(ctx, ids)
		if err != nil {
			return GameBootstrap{}, err
		}
		out.Phase = string(engine.Phase)
		out.CurrentPlayer = engine.CurrentPlayer
		out.PendingReinforcements = engine.PendingReinforcements
		out.Occupy = occupyRequirement(engine.Occupy)
		out.Players = make([]GameBootstrapPlayer, 0, len(engine.Players))
		for i, p := range engine.Players {
			name := names[p.ID]
			if name == "" {
				name = p.ID
			}
			out.Players = append(out.Players, GameBootstrapPlayer{
				UserID:     p.ID,
				UserName:   name,
				Color:      bootstrapColor(i),
				CardCount:  len(p.Cards),
				Eliminated: p.Eliminated,
			})
		}
		tb, err := json.Marshal(engine.Territories)
		if err != nil {
			return GameBootstrap{}, err
		}
		out.Territories = tb
		if s.gameEvent != nil {
			events, err := s.gameEvent.ListGameEvents(ctx, s.db.Queryer(), g.ID, 250)
			if err != nil {
				return GameBootstrap{}, err
			}
			out.Events = make([]GameEventEntry, 0, len(events))
			for _, ev := range events {
				out.Events = append(out.Events, GameEventEntry{
					ID:          ev.ID,
					GameID:      ev.GameID,
					ActorUserID: ev.ActorUserID,
					EventType:   ev.EventType,
					Body:        ev.Body,
					CreatedAt:   ev.CreatedAt,
				})
			}
		}
		return out, nil

	default:
		return GameBootstrap{}, ErrInvalidGameInput
	}
}

func (s *GamesService) userNamesByIDs(ctx context.Context, ids []string) (map[string]string, error) {
	return s.userNamesByIDsQ(ctx, s.db.Queryer(), ids)
}

func (s *GamesService) userNamesByIDsQ(ctx context.Context, q db.Querier, ids []string) (map[string]string, error) {
	if len(ids) == 0 {
		return map[string]string{}, nil
	}
	if q == nil {
		out := make(map[string]string, len(ids))
		for _, id := range ids {
			out[id] = id
		}
		return out, nil
	}
	rows, err := q.Query(
		ctx,
		`SELECT id::text, username FROM users WHERE id::text = ANY($1::text[])`,
		ids,
	)
	if err != nil {
		return nil, err
	}
	if rows == nil {
		out := make(map[string]string, len(ids))
		for _, id := range ids {
			out[id] = id
		}
		return out, nil
	}
	defer rows.Close()

	out := make(map[string]string, len(ids))
	for rows.Next() {
		var id, username string
		if err := rows.Scan(&id, &username); err != nil {
			return nil, err
		}
		out[id] = username
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func containsID(ids []string, target string) bool {
	for _, id := range ids {
		if id == target {
			return true
		}
	}
	return false
}

func bootstrapColor(idx int) string {
	palette := []string{"#ef4444", "#3b82f6", "#22c55e", "#f59e0b", "#a855f7", "#06b6d4"}
	if idx < 0 {
		return palette[0]
	}
	return palette[idx%len(palette)]
}

func decodeLobbyState(raw json.RawMessage) (lobbyState, error) {
	var lobby lobbyState
	if err := json.Unmarshal(raw, &lobby); err != nil {
		return lobbyState{}, ErrInvalidGameInput
	}
	if lobby.PlayerCount < 3 || lobby.PlayerCount > 6 || len(lobby.PlayerIDs) == 0 || len(lobby.PlayerIDs) > lobby.PlayerCount {
		return lobbyState{}, ErrInvalidGameInput
	}
	seen := make(map[string]struct{}, len(lobby.PlayerIDs))
	for _, pid := range lobby.PlayerIDs {
		if pid == "" {
			return lobbyState{}, ErrInvalidGameInput
		}
		if _, ok := seen[pid]; ok {
			return lobbyState{}, ErrInvalidGameInput
		}
		seen[pid] = struct{}{}
	}
	return lobby, nil
}

func isLegacyUninitializedSetup(g risk.Game) bool {
	if g.Phase != risk.PhaseSetupClaim && g.Phase != risk.PhaseSetupReinforce {
		return false
	}
	if len(g.Players) < 3 || len(g.Players) > 6 {
		return false
	}
	return true
}

func mapGameActionErr(err error) error {
	switch {
	case errors.Is(err, risk.ErrOutOfTurn),
		errors.Is(err, risk.ErrInvalidMove),
		errors.Is(err, risk.ErrInvalidPhase):
		return ErrInvalidGameAction
	default:
		return err
	}
}

func occupyRequirement(o *risk.OccupyState) *GameOccupyRequirement {
	if o == nil {
		return nil
	}
	return &GameOccupyRequirement{
		From:    string(o.From),
		To:      string(o.To),
		MinMove: o.MinMove,
		MaxMove: o.MaxMove,
	}
}

func displayName(names map[string]string, userID string) string {
	if userID == "" {
		return "Unknown player"
	}
	if name := strings.TrimSpace(names[userID]); name != "" {
		return name
	}
	return userID
}

func pluralize(noun string, n int) string {
	if n == 1 {
		return noun
	}
	if strings.HasSuffix(noun, "y") && len(noun) > 1 {
		return noun[:len(noun)-1] + "ies"
	}
	return noun + "s"
}

func joinDice(values []int) string {
	if len(values) == 0 {
		return "-"
	}
	parts := make([]string, 0, len(values))
	for _, v := range values {
		parts = append(parts, fmt.Sprintf("%d", v))
	}
	return strings.Join(parts, ", ")
}
