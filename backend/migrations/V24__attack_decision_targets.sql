-- Per-territory attack analytics: which territories a player could attack
-- at each decision point, not just how many. Parallel arrays, one entry per
-- distinct attackable territory (deduplicated across source territories,
-- keeping the best odds): options_targets[i] lies in
-- options_target_continents[i] ('' if in none) and could be taken with
-- probability options_win_probs[i]. to_continent is the chosen target's
-- continent. NULL on rows extracted before V24 (extractor version 1).
ALTER TABLE attack_decisions
    ADD COLUMN to_continent              text,
    ADD COLUMN options_targets           text[],
    ADD COLUMN options_target_continents text[],
    ADD COLUMN options_win_probs         real[];
