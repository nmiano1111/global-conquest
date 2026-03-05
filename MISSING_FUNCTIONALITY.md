# Global Conquest — Missing & Incomplete Functionality

This document audits what is implemented vs. what is not, based on a full read of both the `backend/` and `global-conquest/` source trees.

---

## Summary

The Risk game engine is **fully implemented** in Go — every game action exists and works correctly. The gaps are almost entirely on the **service layer wiring** and **frontend UI** side: several engine capabilities are never exposed to players.

| Priority | Area | Status |
|---|---|---|
| 🔴 Critical | Card trading | Backend engine complete; never exposed to frontend |
| 🔴 Critical | Setup phases (claim + initial placement) | Engine complete; no UI, no service wiring |
| 🔴 Critical | Game over / winner screen | Engine detects winner; frontend ignores it |
| 🟡 High | Game completion persisted to DB | Game status never updated to "completed" |
| 🟡 High | Setup phase service actions | `claim_territory` / `place_initial_army` missing from action dispatch |
| 🟠 Medium | Profile page (edit/change password) | Page exists but is read-only |
| 🟠 Medium | Session/token refresh | Fixed 24-hour TTL, no refresh endpoint |
| ⚪ Low | Game listing filters | No filter by status or player |
| ⚪ Low | Player elimination notification | No explicit event when a player is eliminated |

---

## 1. Card Trading — No Frontend, No Service Wiring

**Severity: Critical**

### What exists
- `backend/internal/risk/engine.go` — `TradeCards()` is fully implemented: validates sets (3 of a kind or one of each), calculates bonus armies (including territory bonuses), adds armies, and manages deck/discard.

### What's missing

**Service layer** (`backend/internal/service/game.go`, action dispatch ~line 335):
```
✅ "place_reinforcement"
✅ "attack"
✅ "occupy"
✅ "end_attack"
✅ "fortify"
✅ "end_turn"
❌ "trade_cards"    ← not in switch, engine function unreachable
```

**Frontend** (`global-conquest/src/router/pages/GamePage.tsx`):
- No card display panel
- No card selection UI
- No `trade_cards` action dispatch

**Effect:** Players accumulate cards (earned after conquering a territory) but can never trade them. Since trading is mandatory when holding 5+ cards, this creates an unresolvable game state.

---

## 2. Setup Phases — Engine Exists, Nothing Else Does

**Severity: Critical**

### What exists
The engine defines and fully implements two setup phases:
- `PhaseSetupClaim` — players take turns claiming one unclaimed territory
- `PhaseSetupReinforce` — players place initial reserve armies on owned territories

Engine methods: `ClaimTerritory()`, `PlaceInitialArmy()` — both complete with validation.

### What's missing

**Game start** (`backend/internal/service/game.go`):
- `JoinClassicGame()` calls `risk.NewClassicAutoStartGame()` when the lobby fills
- This function skips directly to `PhaseReinforce` — setup phases are never entered
- No code path exists to start a game in `PhaseSetupClaim`

**Service action dispatch** (`backend/internal/service/game.go`):
```
❌ "claim_territory"       ← not in switch
❌ "place_initial_army"    ← not in switch
```

**Frontend** (`global-conquest/src/router/pages/GamePage.tsx`):
- `phaseMode` only resolves for: `"reinforce" | "attack" | "occupy" | "fortify"`
- No click handler or UI for setup phases
- If a game were to start in `PhaseSetupClaim`, the board would render with no interactive elements

---

## 3. Game Over — Not Shown, Not Persisted

**Severity: Critical**

### What exists
- `engine.checkWinner()` correctly detects when one player holds all territories
- Sets `game.Phase = PhaseGameOver` and `game.Winner = playerID`
- Called automatically after `EndTurn()` and `OccupyTerritory()`

### What's missing

**Service layer** (`backend/internal/service/game.go`, `ApplyGameAction()`):
- After applying any action, the service does not inspect the resulting phase
- When `PhaseGameOver` is reached, the game's database `status` is never updated to `"completed"`
- No game event is emitted for game completion

**Frontend** (`global-conquest/src/router/pages/GamePage.tsx`):
- No handler or UI branch for `phase === "game_over"`
- No winner announcement overlay or screen
- No "return to lobby" flow after game ends

**Effect:** The engine correctly terminates the game, but from the players' perspective the game simply freezes. There is no winner announcement and the game record stays in `"in_progress"` status forever.

---

## 4. Game Status Never Updated to "Completed"

**Severity: High** (depends on #3)

The `games` table has a `status` column with values `"lobby"`, `"in_progress"`, `"completed"`. The transition `"in_progress"` → `"completed"` is never triggered anywhere in the codebase. The game listing therefore never reflects finished games correctly, and any analytics or history features built on `status = 'completed'` would find no records.

**Fix location:** `backend/internal/service/game.go`, `ApplyGameAction()` — after persisting state, check if `riskGame.Phase == risk.PhaseGameOver` and update `game.Status` to `"completed"`.

---

## 5. Player Elimination Has No Dedicated Event

**Severity: High**

When a player loses their last territory during `OccupyTerritory()`, the engine sets `playerState.Eliminated = true` and skips that player's turns. No game event of type `"player_eliminated"` is saved, and no WebSocket message is broadcast for this. Other players have no notification; the eliminated player receives no acknowledgment.

---

## 6. Profile Page Is Read-Only

**Severity: Medium**

`ProfilePage.tsx` displays username, user ID, and role — nothing else. There is no ability to:
- Change password
- Change username
- View game history
- See stats (games played, won, etc.)

The backend `UsersService` has no update-user endpoint. No HTTP route exists for `PUT /api/users/:id`.

---

## 7. No Session Refresh / Token Renewal

**Severity: Medium**

Session tokens have a fixed 30-day TTL (backend) but the frontend stores and uses them with no refresh mechanism:
- No `POST /api/auth/refresh` endpoint exists
- When a token expires, the next request returns 401, and the frontend clears the session and redirects to login
- There is no proactive renewal before expiry

For a long multiplayer game session this is acceptable (30 days is generous), but if a player's session expires mid-game they are silently disconnected with no in-game message.

---

## 8. Game Listing Has No Filters

**Severity: Low**

`GET /api/games` returns all games. There is no way to filter by:
- `status` (lobby / in_progress / completed)
- Games the current user is a member of
- Games available to join

The lobby page shows all games, which will become unwieldy as the number of games grows.

---

## 9. No Spectator Mode

**Severity: Low**

There is no mechanism for a non-participant to observe a game in progress. The WebSocket game state is broadcast only to registered players (clients in the game's player list). An observer visiting a game URL would get the initial bootstrap state but no real-time updates.

---

## 10. Game Chat Not Visible in Lobby / Game History

**Severity: Low**

- `GET /api/games/:id/bootstrap` includes past game events and game chat history
- There is no standalone endpoint to page through game chat history independently
- The lobby chat (`GET /api/chat/lobby/messages`) does not support pagination — it returns all messages

---

## Appendix: Feature Matrix

| Feature | Engine | Service | WebSocket | Frontend | Verdict |
|---|---|---|---|---|---|
| User registration & login | — | ✅ | — | ✅ | Complete |
| Lobby: create / join game | — | ✅ | ✅ | ✅ | Complete |
| Lobby chat + typing indicators | — | ✅ | ✅ | ✅ | Complete |
| Setup: claim territory | ✅ | ❌ | ❌ | ❌ | **Missing** |
| Setup: place initial armies | ✅ | ❌ | ❌ | ❌ | **Missing** |
| Reinforce phase | ✅ | ✅ | ✅ | ✅ | Complete |
| Attack phase | ✅ | ✅ | ✅ | ✅ | Complete |
| Occupy territory | ✅ | ✅ | ✅ | ✅ | Complete |
| Fortify phase | ✅ | ✅ | ✅ | ✅ | Complete |
| End turn | ✅ | ✅ | ✅ | ✅ | Complete |
| Card trading | ✅ | ❌ | ❌ | ❌ | **Missing** |
| Game over detection | ✅ | ⚠️ | ⚠️ | ❌ | **Incomplete** |
| Persist game completion | — | ❌ | — | — | **Missing** |
| Player elimination event | ✅ | ❌ | ❌ | ❌ | **Missing** |
| Event log | — | ✅ | ✅ | ✅ | Complete |
| Game chat | — | ✅ | ✅ | ✅ | Complete |
| Admin: user management | — | ✅ | — | ✅ | Complete |
| Profile: view | — | ✅ | — | ✅ | Complete |
| Profile: edit / change password | — | ❌ | — | ❌ | **Missing** |
| Session refresh | — | ❌ | — | ❌ | **Missing** |
| Spectator mode | — | ❌ | ❌ | ❌ | **Missing** |
| Game listing filters | — | ❌ | — | ❌ | **Missing** |
