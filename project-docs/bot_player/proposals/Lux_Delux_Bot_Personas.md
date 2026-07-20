# Lux Delux-inspired bot personas

Six `bot.Strategy` implementations ported from Lux Delux's built-in AIs (see
`Lux_Delux_AI_Research_Notes.md` for why, `Lux_Port_Notes.md` for how each
one was adapted to this engine's architecture). Registry IDs follow the
`<name>-v1` convention and are usable anywhere a strategy ID appears
(`--strategies` on `cmd/tournament`/`cmd/tdtraindata`, a bot's
`PlayerState.Strategy`).

## `angry-v1` — Angry

The simplest and most aggressive persona: no continent or cluster
awareness at all, every decision is a purely local, greedy comparison over
a territory's immediate neighbors.

- **Reinforce**: places every pending army on whichever owned territory
  borders the most enemy territories.
- **Attack**: attacks the first owned territory it finds (in board order)
  that outnumbers its weakest enemy neighbor at all — a bare one-army
  edge is enough, no odds-awareness.
- **Cards**: hoards them, only trading when forced at 5+ in hand.
- **Occupy/Fortify**: keeps the more-threatened side defended, moving the
  legal minimum or maximum accordingly.

In practice: attacks constantly and indiscriminately, racking up captures
but leaving itself overextended and thin.

## `cluster-v1` — Cluster

Expands outward from its largest owned landmass without regard for
continent identity, via a staged attack sequence.

- **Root selection**: the most valuable continent it fully owns, or (if
  none) its single biggest army stack.
- **Attack** (in order, first qualifying stage wins each turn):
  1. **Easy-expand** — take a border territory's sole enemy neighbor if
     beatable.
  2. **Fill-out** — kill any enemy "island" fully surrounded by owned
     territory.
  3. **Consolidate** — merge two or more border territories that share
     the same lone enemy neighbor, attacking with their combined force.
  4. **Split-up** — if a border territory's armies exceed its combined
     enemy neighbors by 1.2×, attack the weakest one.
  5. **Hogwild/stalemate** — if it outnumbers every other player combined
     (or has amassed 1500+ armies anywhere), broaden the same scan to
     every owned territory.
- **Reinforce**: reinforces its cluster's weakest border, or routes
  toward the easiest continent to take if it owns none outright.
- **Cards**: forced-only, like Angry.

The methodical, expansionist baseline the other Cluster-derived personas
(Quo, Boscoe) build on.

## `pixie-v1` — Pixie

Continent-economy driven: each turn it re-evaluates which continents it
can plausibly take and hold, and commits reinforcement/attacks to just
those.

- **Wanted continents**: any positive-bonus continent where its own +
  adjoining armies already outweigh the enemy armies present.
- **Reinforce**: reinforces the weakest border of a wanted-but-undefended
  continent (a border under 20 armies counts as undefended); if its
  wanted continents are all healthy, spreads reinforcements near enemies
  instead.
- **Attack**: attacks within its wanted continents first (any positive
  edge qualifies), then looks for the single best-ratio matchup anywhere
  on the board to grab a card, then hogwild/stalemate if dominant.
- **Occupy**: a multi-level tie-break — favors whichever side is more
  directly threatened, falling back to continent-membership, then to
  counting threats within wanted continents.

Plays a tighter, more defensible game than Cluster, trading raw expansion
for holding what it takes.

## `quo-v1` — Quo

Nearly identical to Cluster (Quo/Shaft override almost nothing from it in
Lux's own source) plus one new trick: a forward-sweep lookahead, and an
always-cash card policy.

- **Attack sequence**: Cluster's easy-expand → fill-out → consolidate,
  then (only if nothing's been conquered yet this turn) a **sweep**: from
  a border territory, repeatedly fold any frontier territory with exactly
  one un-seen enemy neighbor into the "seen" set; if the frontier
  collapses to a single remaining contact point, attack toward it. Then
  falls through to a best-ratio card grab and hogwild/stalemate, same as
  Pixie.
- **Cards**: always cashes the first legal set, unlike Cluster's
  forced-only policy.
- **Placement/occupy/fortify**: identical to Cluster's.

The sweep lets Quo recognize when a messy border is about to collapse
into a clean chokepoint and push through it, rather than attacking
piecemeal.

## `boscoe-v1` — Boscoe

Cluster with a defensive trigger: when one player becomes dominant
(≥50% of armies, income, or territory), Boscoe drops everything to try to
eliminate them.

- **Dominant-player check**: recomputed every turn — is any other player
  at or above half the board's total armies, reinforcement income, or
  territory count?
- **If someone's dominant**: routes reinforcement and attacks toward the
  cheapest reachable path into whichever of their continents has the
  biggest bonus, via a weighted shortest-path search — the same
  "recompute fresh, one hop at a time" approach used throughout this
  port rather than pre-planning a full route.
- **Otherwise**: falls back to a *restrained* version of Cluster's attack
  sequence — easy-expand, fill-out, consolidate, but deliberately never
  split-up — plus a best-ratio card grab and hogwild/stalemate.
- **Cards**: always cashes, like Quo.

The project's "check the leaderboard" persona — mostly plays a normal
expansion game, but reacts hard the moment someone starts running away
with it.

## `killbot-v1` — Killbot

The most aggressive elimination hunter: rather than reacting to a
dominant leader, Killbot actively looks for a weak rival to wipe out the
moment it's safely ahead.

- **Kill-target check**: among living rivals it can plausibly reach (its
  single strongest attacking stack would beat that rival's whole army +
  territory count) and is comfortably stronger than (its own total
  armies exceed 2× the rival's — discounted for the reinforcements
  they'd get from cashing their next card set), picks the weakest one.
- **If a target qualifies**: routes reinforcement and attacks toward the
  cheapest path into *any* of their territory (not continent-scoped, just
  "get to them"), the same single-hop router Boscoe uses for continents.
- **Otherwise**: falls back to Pixie's continent-economy logic wholesale
  (placement, occupy, fortify, and attack) — Killbot's Lux source is
  backed by a Pixie variant for everything except the kill mechanic.
- **Cards**: always cashes, inherited from its Pixie backer.

The one persona built to snowball: once it's ahead, it stays on offense
against whoever's most likely to become a problem later.

## Playing them against each other

```bash
go run ./cmd/tournament \
  --strategies angry-v1,cluster-v1,pixie-v1,quo-v1,boscoe-v1,killbot-v1 \
  --games 20
```

Six is `cmd/tournament`'s max seat count — for a diversified matchup that
also includes `basic-v1`/`scored-v1`, drop one of the above. See
`cmd/tournament/README.md` for full flag documentation.
