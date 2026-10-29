package main

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
)

// migrationName matches Flyway-style versioned migrations: V<n>__<desc>.sql.
var migrationName = regexp.MustCompile(`^V(\d+)__.+\.sql$`)

// orderMigrations returns names sorted by their numeric version, so V10__
// runs after V9__ (a plain string sort puts it before V2__, which breaks a
// fresh database). It returns an error for any name that isn't a versioned
// migration or that reuses another file's version, since either means the
// run order is ambiguous.
func orderMigrations(names []string) ([]string, error) {
	type migration struct {
		version int
		name    string
	}
	migrations := make([]migration, 0, len(names))
	byVersion := make(map[int]string, len(names))
	for _, name := range names {
		m := migrationName.FindStringSubmatch(name)
		if m == nil {
			return nil, fmt.Errorf("migration %q is not named V<version>__<description>.sql", name)
		}
		version, err := strconv.Atoi(m[1])
		if err != nil {
			return nil, fmt.Errorf("migration %q: bad version: %w", name, err)
		}
		if other, dup := byVersion[version]; dup {
			return nil, fmt.Errorf("migrations %q and %q share version %d", other, name, version)
		}
		byVersion[version] = name
		migrations = append(migrations, migration{version: version, name: name})
	}
	sort.Slice(migrations, func(i, j int) bool { return migrations[i].version < migrations[j].version })

	out := make([]string, len(migrations))
	for i, m := range migrations {
		out[i] = m.name
	}
	return out, nil
}
