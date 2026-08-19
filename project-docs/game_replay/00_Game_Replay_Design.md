# Global Conquest Game Replay — Design

## Purpose

Let a player open any game (in progress or finished) and watch its actions play back, in order, up to whatever has actually happened so far — territories changing hands, armies moving, combat resolving — the same way a live spectator would see it happen, just after the fact and at a pace the viewer controls.

This document is a design proposal, not an implementation plan. Nothing here is built yet.

## Core Principle

Replay must reuse the same code path that already turns one committed action into visible change for a live viewer, fed from storage instead of from the socket. It must not become a second, parallel implementation of "how a turn looks," and it must never be able to influence the actual live game.

## Goals

1. Replay a game's full action history, in order, using the same visual/animation behavior a live spectator already sees (board changes, dice-roll display, capture/combat highlights, event log lines).
2. Support games still in progress — replay always shows "everything committed so far," not just finished games.
3. Give the viewer play/pause/step/speed controls rather than reproducing real elapsed time between actions.
4. Guarantee, structurally, that viewing replay can never result in a live game action being submitted from stale/historical state.

## Non-Goals (for this phase)

- Backfilling replay for games created before this ships (`games.event_history_complete` already establishes the precedent that older games simply don't get a feature that depends on capture starting from day one).
- Reproducing literal wall-clock gaps between actions.
- Live-tailing an open replay as new actions arrive elsewhere in the same game. Replay is a static snapshot of "everything committed as of when you opened it"; watching a still-in-progress game further means re-opening it. Exiting replay (navigating back to `/games/:id`) re-mounts `GamePage`, which re-bootstraps fresh — so leaving replay always snaps straight back to whatever the game's actual current state is, never to anything replay was holding.
- Any bot-ML/analytics use case — this is scoped as a player-facing feature. (The existing `game_domain_events` combat-roll log stays exactly as-is for the reporting pipeline; this is additive, not a replacement.)

---

## Current State

Three pieces of infrastructure already exist, built for other purposes, and matter here:

- **`game_events`** (`backend/internal/store/game_event.go`) — a free-text, human-readable sentence persisted for every action (`GamesService.ApplyGameAction`, `service/game.go:~1102-1364`: "Alice placed 3 armies on Brazil.", etc). Already fetched on bootstrap (last 250) and live-appended in `GamePage.tsx`. Good context, not sufficient alone — it has no board-state data.
- **`game_domain_events`** (`backend/internal/store/game_domain_event.go`) — a proper event-sourcing table: append-only, per-game monotonic `game_sequence` (via `games.event_sequence`), versioned typed JSON payloads. Currently wired for exactly one thing: `combat_roll_resolved`, emitted only from the Attack path in `engine.go:697`, consumed only by the `reporting` package for win-rate/streak analytics. No HTTP exposure.
- **`wsmsg.GameStateUpdatedPayload`** (`game/server.go:706-745`) — the actual `game_state_updated` broadcast, built from every `GameActionUpdate` regardless of action type. It already carries everything needed to render one turn's worth of change: `phase`, `current_player`, `pending_reinforcements`, `occupy`, `players`, `territories`, `result` (dice/losses, when applicable), `event` (the same sentence as above), and `action`/`action_territory`/`action_from`/`action_to`.
- **`GamePage.tsx`'s `on("game_state_updated", ...)` handler** (~lines 218-336) is where that payload actually becomes visible change: it updates board state, sets `diceResult` for the roll popup, sets `lastActionTerritory/From/To/Type` (which drive `GameMap`'s highlight props), and appends the event log line. This is the literal mechanism that makes a bot's turn look right today.

The insight this design leans on: **that handler is already "replay one action," it just happens to be fed by a socket.** Nothing new needs to be invented for the animation/visual layer — it needs to be extracted and fed from storage instead.

---

## Data Model

New table, `game_replay_events`:

```sql
CREATE TABLE game_replay_events (
    id              uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    game_id         uuid        NOT NULL REFERENCES games(id) ON DELETE CASCADE,
    game_sequence   bigint      NOT NULL,
    occurred_at     timestamptz NOT NULL DEFAULT now(),
    actor_player_id uuid,
    action_type     text        NOT NULL,
    payload         jsonb       NOT NULL,  -- same shape as GameStateUpdatedPayload
    UNIQUE (game_id, game_sequence)
);
CREATE INDEX game_replay_events_game_seq_idx ON game_replay_events(game_id, game_sequence);
```

`payload` is field-for-field the same JSON shape already sent as `game_state_updated` (minus `ActorCards`, which is private to the acting player and never part of the broadcast anyway — it's already sent separately via `your_cards`). No new derivation logic: it's a direct persist of data already assembled in `ApplyGameAction` for the broadcast and the text log.

`games.event_sequence` is generalized from "ticks only on Attack" (its only current use, feeding `game_domain_events`) to "ticks on every committed action." The insert happens inside `GamesService.ApplyGameAction`'s existing transaction, at the same point `SaveGameEvent`/`InsertDomainEvent` already run — same atomicity guarantee, no new locking concerns (`games` row is already `SELECT ... FOR UPDATE`'d there). This makes `game_sequence` a true universal per-game action counter, and every action now correlates a `game_events` row, an optional `game_domain_events` row (attacks only), and exactly one `game_replay_events` row by the same sequence number.

## Backend API

`GET /api/games/:id/replay?after=<sequence>&limit=<n>` — read-only, cursor-paginated over `game_replay_events`, ordered by `game_sequence`. Returns `{ sequence, occurred_at, actor_player_id, action_type, payload }[]`.

Bootstrap gains one field: `replay_available: bool` (true once a game has ≥1 row — true for every game from the moment it's created, going forward; always false for pre-existing games, matching the `event_history_complete` precedent).

Nothing here is a new mutation surface. This endpoint cannot be used to change game state — it only reads.

## Frontend Architecture

1. Extract the body of `GamePage.tsx`'s `game_state_updated` handler into a pure function:

   ```ts
   function applyGameStateUpdate(
     prevGame: GameBootstrap,
     payload: GameStateUpdatedPayload
   ): {
     nextGame: GameBootstrap;
     diceResult?: DiceRollResult;
     lastActionTerritory: string;
     lastActionFrom: string;
     lastActionTo: string;
     lastActionType: string;
     eventMessage?: GameEventMessage;
   }
   ```

   The live handler becomes a thin wrapper: unwrap the socket message, call this, apply the returned pieces to state. No behavior change for live play — this is a refactor, not a rewrite.

2. New route, `/games/:id/replay`, fetches the stored payload sequence once on open and holds its own local state (`replayGame`, `replayIndex`, `playing`, `speed`). Advancing a step calls `applyGameStateUpdate` with the next stored payload and renders the result through the existing `GameMap` component (which already just takes a `GameBootstrap`-shaped prop — it doesn't know or care whether that came from a socket, a REST call, or a replay loop).

3. Playback controls: play/pause, step forward/back, speed multiplier (instant / 1x-ish / slow). No attempt to reproduce real elapsed time between actions (decided — see conversation).

---

## Live/Replay Isolation — Hard Invariant

**A player must never be able to submit a live game action derived from replay's rewound state.** This is a hard requirement, not a nice-to-have, and it's a *frontend* problem, not primarily a security one: the engine only ever mutates from the current authoritative `games.state` row it loads fresh (`GetByIDForUpdate`) inside each transaction — it never trusts client-supplied board state, only client-supplied intent (territory names, army counts). So even in the worst case, a stale-informed action either gets rejected as illegal against current state or produces a result the player didn't expect — it can never corrupt the actual game. The thing we're actually preventing is a confused/wrong submission and a broken mental model, not data corruption.

**Assumption, stated explicitly:** "pause live gameplay" is read here as *this viewer's own client* being unable to submit actions while replay is open — not a global freeze of the match for every player. Spectators (or the current player, mid-turn) opening replay shouldn't be able to stop anyone else's clock. If that's not the intent, this needs to be revisited before implementation, since a real cross-client pause is a materially different (and much stranger, multiplayer-wise) feature.

Mechanisms, in order of how load-bearing they are:

1. **Route-level separation is the primary guarantee, and it's structural, not a flag.** Replay lives at its own route (`/games/:id/replay`), a different component tree from `GamePage`. The action-submission panel, the `selectedTerritory/From/To` selection state, and every WS "send game action" call site simply do not exist inside that tree. There is no code path by which replay's view can submit a game action, because the code that submits game actions isn't mounted. This is deliberately *not* a modal, split-view, or in-place toggle on top of the live board — any of those would keep the live action UI mounted in the same tree as the historical view, turning "can't submit stale actions" into "must remember to check a flag everywhere" instead of "is architecturally impossible."
2. **Replay never subscribes to the live broadcast channel.** It sources exclusively from `GET /games/:id/replay`, fetched once at open time (see Open Questions on live-tailing). It does not listen for `game_state_updated` at all, so there's no path for a live broadcast and a replay step to race into the same state variable.
3. **`replayGame` and `game` are separate variables of the same type, never cross-assigned.** `applyGameStateUpdate` is pure — it takes a `GameBootstrap` and a payload and returns a new `GameBootstrap`; it has no idea which state slot its caller puts the result in. The live handler and the replay loop each own their own slot.
4. **Leaving the replay route and returning to `/games/:id`** re-mounts `GamePage` fresh — it re-runs its normal bootstrap fetch and re-establishes its own live subscription. It does not resume from anything replay was holding.

Net effect: submitting a game action while looking at replay isn't "blocked by a check," it's "there is no button, no handler, and no socket subscription that could do it."

## Open Questions

- **Entry point placement.** A "Watch Replay" link/button on `GamePage` (gated on `replay_available`) that navigates to the new route is the natural fit, but exact placement/wording isn't decided.
- **Whether replay needs its own permission check**, or just inherits whatever access rule already gates `GET /games/:id/bootstrap` (`CanAccessGame`). Current assumption: same rule, no new access model.
