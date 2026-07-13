# Global Conquest AI Heuristic Strategy Framework

## Purpose

This document describes the next evolution of the Global Conquest bot
from a simple rule-based player into a configurable heuristic engine. It
is intentionally focused on **decision making**, not engine rules. The
authoritative game engine remains responsible for legality, state
transitions, combat resolution, card validation, and every other game
rule.

The bot's responsibility is to evaluate *legal* actions and choose the
one that best advances its strategic objectives.

------------------------------------------------------------------------

# Core Philosophy

The engine owns **what is legal**.

The bot owns **which legal action is best**.

Every decision should follow the same pipeline:

1.  Generate all legal actions from the engine.
2.  Extract features for every candidate.
3.  Score every candidate.
4.  Apply strategic weighting.
5.  Choose the highest scoring action.
6.  Submit the command through the normal authoritative application
    path.

The bot should never reconstruct legality by inspecting raw game state.

------------------------------------------------------------------------

# Candidate Action Scoring

Avoid hard-coded "if/else" strategies where possible.

Instead, every phase should become a candidate scoring problem.

Example:

    Legal attacks

    Brazil -> North Africa
    Ukraine -> Ural
    Peru -> Venezuela
    End Attack Phase

Every option---including ending the phase---receives a score.

The highest scoring option wins.

This architecture scales naturally from simple heuristics to machine
learning.

------------------------------------------------------------------------

# Explainable Decisions

Every selected action should include an explanation.

Example:

    Attack:
    Brazil -> North Africa

    Final Score: 18.2

    +8.0 Army advantage
    +6.0 Breaks enemy continent
    +4.0 Card opportunity
    -2.0 Expected losses
    +2.2 Weak target

This should be logged for debugging and eventually become
analytics/training data.

------------------------------------------------------------------------

# Reinforcement Strategy

The first heuristic upgrade should score every legal reinforcement
destination.

Useful features include:

-   Adjacent enemy armies
-   Number of adjacent enemy territories
-   Friendly armies already present
-   Border vs interior
-   Fraction of continent already owned
-   Whether the territory is a continent gateway
-   Strategic vulnerability

General principles:

Reward:

-   Threatened borders
-   Near-complete continents
-   Important choke points
-   Territories that unlock future attacks

Penalize:

-   Safe interior territories
-   Excessive concentration of armies
-   Reinforcing already dominant positions when other borders are weak

The objective is to maximize the strategic value of every reinforcement
rather than simply reinforcing the weakest territory.

------------------------------------------------------------------------

# Attack Strategy

Attack selection is expected to provide the largest quality improvement.

Every legal attack should be evaluated.

Potential features:

-   Army advantage
-   Estimated capture probability
-   Expected attacker losses
-   Expected defender losses
-   Completes a continent
-   Breaks an enemy continent
-   Eliminates a player
-   Earns first card this turn
-   Exposure created after the attack
-   Number of new borders created

The strategy should no longer simply use:

    source >= target + 2

Instead it should weigh:

-   Expected reward
-   Expected cost
-   Strategic consequences

------------------------------------------------------------------------

# End Attack Is A Candidate

Do not treat ending the attack phase as special logic.

Instead generate it as another legal candidate.

    Attack A
    Attack B
    Attack C
    End Attack Phase

This allows personalities and difficulty to naturally influence
aggression simply by adjusting the score assigned to ending the phase.

------------------------------------------------------------------------

# Occupation Strategy

Avoid simplistic rules such as:

-   Always move minimum
-   Always move everything

Instead evaluate:

Source territory:

-   Remaining border pressure
-   Nearby enemy strength

Destination territory:

-   Future attack opportunities
-   Defensive requirements
-   Continent importance

General principle:

Leave enough strength behind to avoid creating an obvious weakness while
maintaining offensive momentum.

------------------------------------------------------------------------

# Fortification Strategy

Fortification should move strength from low-value areas to high-value
areas.

Useful features:

-   Interior vs border territory
-   Threat level
-   Strategic importance
-   Continent defense
-   Gateway value
-   Source exposure after movement

General objective:

Move armies from safe interior territories toward the weakest important
border.

------------------------------------------------------------------------

# Card Strategy

The current implementation may simply turn in whenever possible.

Future strategy should distinguish between:

Mandatory turn-in

Optional turn-in because:

-   It enables a continent
-   It enables an elimination
-   The player is under pressure
-   Cards are approaching the limit

Optional delay because:

-   No immediate benefit exists
-   Holding cards creates a stronger future opportunity

The engine continues to determine legality and reinforcement values.

The bot only decides whether to turn in and which legal set to choose.

------------------------------------------------------------------------

# Continent Evaluation

Every continent should receive a dynamic value.

Possible inputs:

-   Bonus value
-   Fraction owned
-   Enemy presence
-   Number of borders
-   Number of opponents
-   Defensive cost

The bot should avoid simplistic rules such as "always pursue Australia."

Instead it should determine whether a continent is strategically
worthwhile in the current board state.

------------------------------------------------------------------------

# Player Elimination

Eliminations are often worth pursuing because of card rewards.

Evaluate:

-   Opponent card count
-   Remaining territories
-   Remaining armies
-   Estimated cost
-   Strategic position after elimination

The bot should not blindly chase eliminations across the board.

------------------------------------------------------------------------

# Difficulty

Difficulty should not create different rule systems.

Instead maintain one strategy implementation whose weights vary.

Examples:

-   Aggression
-   Continent priority
-   Exposure penalty
-   Card value
-   Elimination value
-   Baseline preference for ending attack phase

This also naturally supports personalities.

------------------------------------------------------------------------

# Personalities

Personality should primarily be represented by different weight
configurations.

Examples:

Aggressive: - Higher attack value - Lower exposure penalty

Defensive: - Strong exposure penalty - Higher value for ending attacks

Expansionist: - Higher continent value

Opportunist: - Higher elimination and card value

This avoids branching strategy logic.

------------------------------------------------------------------------

# Machine Learning Compatibility

The heuristic framework should be designed so the scoring function can
eventually be replaced or augmented by learned models.

The surrounding architecture should remain unchanged.

Future pipeline:

    Generate legal actions
            ↓
    Extract features
            ↓
    Score actions
            ↓
    Select best
            ↓
    Submit authoritative command

Initially:

-   Hand-written heuristics

Later:

-   Logistic regression
-   Gradient boosted trees
-   Neural evaluation
-   Monte Carlo search
-   Reinforcement learning

The action selection architecture should not need to change.

------------------------------------------------------------------------

# Analytics

Record enough information to understand why the bot behaves as it does.

Future analytics may include:

-   Candidate actions considered
-   Winning action
-   Score
-   Score components
-   Strategy version
-   Phase
-   Top alternative actions

This data will support:

-   Debugging
-   Strategy tuning
-   Performance comparisons
-   Supervised learning
-   Explainability

------------------------------------------------------------------------

# Recommended Implementation Order

1.  Introduce candidate-action scoring architecture.
2.  Improve attack evaluation first.
3.  Improve reinforcement evaluation.
4.  Improve occupation heuristics.
5.  Improve fortification heuristics.
6.  Improve card timing.
7.  Add continent valuation.
8.  Add elimination heuristics.
9.  Introduce configurable difficulty weights.
10. Add personalities through weight profiles.
11. Begin collecting explainable decision data.
12. Use analytics to refine heuristics before introducing machine
    learning.

The immediate objective is not to build a perfect player. It is to build
an architecture that supports continuous, measurable improvement over
time while keeping the authoritative engine completely independent from
strategy.
