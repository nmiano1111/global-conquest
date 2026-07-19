package main

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRawWriterRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.jsonl")
	w, err := newRawWriter(path)
	if err != nil {
		t.Fatalf("newRawWriter: %v", err)
	}

	want := trainingRow{GameID: "g1", Seed: 7, PlayerID: "p0", StrategyID: "scored-v1", Turn: 3, Won: true, Features: []float64{0.5, 1, 0}}
	if err := w.write(want); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := w.close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	if !scanner.Scan() {
		t.Fatal("expected one line of output")
	}
	var got trainingRow
	if err := json.Unmarshal(scanner.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.GameID != want.GameID || got.Seed != want.Seed || got.PlayerID != want.PlayerID || got.Won != want.Won || len(got.Features) != len(want.Features) {
		t.Errorf("round-tripped row = %+v, want %+v", got, want)
	}
}

func TestWriteFeatureNamesProducesValidJSON(t *testing.T) {
	dir := t.TempDir()
	output := filepath.Join(dir, "data.jsonl")

	if err := writeFeatureNames(output); err != nil {
		t.Fatalf("writeFeatureNames: %v", err)
	}

	data, err := os.ReadFile(featureNamesPath(output))
	if err != nil {
		t.Fatalf("read feature names file: %v", err)
	}
	var names []string
	if err := json.Unmarshal(data, &names); err != nil {
		t.Fatalf("unmarshal feature names: %v", err)
	}
	if len(names) == 0 {
		t.Fatal("expected a non-empty feature name list")
	}
}

func TestWriteBoardSchemaProducesValidJSON(t *testing.T) {
	dir := t.TempDir()
	output := filepath.Join(dir, "data.jsonl")

	if err := writeBoardSchema(output); err != nil {
		t.Fatalf("writeBoardSchema: %v", err)
	}

	data, err := os.ReadFile(boardSchemaPath(output))
	if err != nil {
		t.Fatalf("read board schema file: %v", err)
	}
	var schema struct {
		Order []string `json:"order"`
		Edges [][2]int `json:"edges"`
	}
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatalf("unmarshal board schema: %v", err)
	}
	if len(schema.Order) == 0 {
		t.Fatal("expected a non-empty Order list")
	}
	if len(schema.Edges) == 0 {
		t.Fatal("expected a non-empty Edges list")
	}
}
