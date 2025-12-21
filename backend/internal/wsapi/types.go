package wsapi

type ClientMsg struct {
	Type string `json:"type"`
	// Add fields as needed (e.g. room_id, move, etc.)
}

type ServerMsg struct {
	Type string `json:"type"`
}
