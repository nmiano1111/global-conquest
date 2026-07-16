package main

import (
	"encoding/json"
	"os"
)

// rawWriter appends one compact JSON-encoded trainingRow per line (JSONL)
// -- same shape as cmd/tournament's own rawWriter, kept as an independent
// copy since each cmd/* binary in this project is self-contained.
type rawWriter struct {
	f   *os.File
	enc *json.Encoder
}

func newRawWriter(path string) (*rawWriter, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	return &rawWriter{f: f, enc: json.NewEncoder(f)}, nil
}

func (r *rawWriter) write(row trainingRow) error {
	return r.enc.Encode(row)
}

func (r *rawWriter) close() error {
	return r.f.Close()
}
