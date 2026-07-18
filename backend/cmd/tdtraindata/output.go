package main

import (
	"encoding/json"
	"os"
	"strings"

	"github.com/nmiano1111/global-conquest/backend/internal/risk"
	"github.com/nmiano1111/global-conquest/backend/internal/tdstate"
)

// rawWriter appends one compact JSON-encoded trainingRow per line (JSONL)
// -- same shape as cmd/traindata's own rawWriter, kept as an independent
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

// featureNamesPath derives the sidecar path for output's column names --
// e.g. "data.jsonl" -> "data.featurenames.json". Written once per run
// (not once per row): tdstate.FeatureNames' ~400-dimensional output would
// otherwise repeat its full set of string keys on every single row for
// no reason, since the flat Features vector's column order is already
// fixed and identical across every row this tool ever writes (one board,
// the classic map -- see tdstate.FeatureNames' own doc comment on custom
// maps changing this in the future).
func featureNamesPath(output string) string {
	if idx := strings.LastIndex(output, "."); idx > 0 {
		return output[:idx] + ".featurenames.json"
	}
	return output + ".featurenames.json"
}

// writeFeatureNames writes the column names matching every trainingRow's
// Features order to featureNamesPath(output).
func writeFeatureNames(output string) error {
	names := tdstate.FeatureNames(risk.ClassicBoard())
	data, err := json.Marshal(names)
	if err != nil {
		return err
	}
	return os.WriteFile(featureNamesPath(output), data, 0o644)
}
