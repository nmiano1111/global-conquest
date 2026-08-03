-- maps: admin-authored custom board definitions (mapgen.MapDefinition,
-- serialized as-is: continents/adjacency/order, a per-territory layout, and
-- the generation spec used to build it). games.map_id is nullable and
-- defaults to null, meaning the classic board (risk.ClassicBoard()) --
-- every existing game row is unaffected by this migration.
CREATE TABLE maps (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  owner_user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  name text NOT NULL,
  definition jsonb NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX maps_owner_user_id_idx ON maps(owner_user_id);

ALTER TABLE games
  ADD COLUMN IF NOT EXISTS map_id uuid NULL REFERENCES maps(id) ON DELETE RESTRICT;

CREATE INDEX games_map_id_idx ON games(map_id);
