package main

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestOrderMigrations_SortsNumerically(t *testing.T) {
	got, err := orderMigrations([]string{"V10__ten.sql", "V2__two.sql", "V1__one.sql", "V21__maps.sql", "V9__nine.sql"})
	if err != nil {
		t.Fatalf("orderMigrations: %v", err)
	}
	want := []string{"V1__one.sql", "V2__two.sql", "V9__nine.sql", "V10__ten.sql", "V21__maps.sql"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestOrderMigrations_RejectsBadNames(t *testing.T) {
	cases := map[string][]string{
		"no version prefix": {"V1__one.sql", "init.sql"},
		"missing separator": {"V1_one.sql"},
		"lowercase v":       {"v1__one.sql"},
		"duplicate version": {"V3__a.sql", "V3__b.sql"},
		"zero-padded clash": {"V3__a.sql", "V03__b.sql"},
	}
	for name, names := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := orderMigrations(names); err == nil {
				t.Fatalf("expected error for %v", names)
			}
		})
	}
}

// The real migrations directory must always order cleanly and numerically.
func TestOrderMigrations_RepoMigrations(t *testing.T) {
	entries, err := os.ReadDir("../../migrations")
	if err != nil {
		t.Fatalf("read migrations dir: %v", err)
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	ordered, err := orderMigrations(names)
	if err != nil {
		t.Fatalf("orderMigrations: %v", err)
	}
	if len(ordered) == 0 || !strings.HasPrefix(ordered[0], "V1__") {
		t.Fatalf("expected V1__ first, got %v", ordered)
	}
}
