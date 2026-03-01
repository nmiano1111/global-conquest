CREATE TABLE game_chat_messages (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  game_id uuid NOT NULL REFERENCES games(id) ON DELETE CASCADE,
  sender_client_id text NOT NULL,
  sender_name text NOT NULL,
  body text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX game_chat_messages_game_created_idx ON game_chat_messages(game_id, created_at DESC);
