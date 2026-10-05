package sqlite

import (
	"context"
	"errors"
)

// ErrCheckNotImplemented indicates that database verification has not yet
// been implemented. It is a scaffold error, not a result of a failed check.
var ErrCheckNotImplemented = errors.New("sqlite database check is not implemented")

// CheckReport describes the database schema and bookmark count observed by
// Check. SchemaDifferences is empty when the schema matches the expected
// schema for the latest migration version.
type CheckReport struct {
	SchemaVersion     int      `json:"schema_version"`
	ExpectedVersion   int      `json:"expected_version"`
	BookmarkCount     int64    `json:"bookmark_count"`
	SchemaDifferences []string `json:"schema_differences,omitempty"`
}

// OK reports whether the observed database is at the latest version and its
// schema matches the expected schema.
func (r CheckReport) OK() bool {
	return r.SchemaVersion == r.ExpectedVersion && len(r.SchemaDifferences) == 0
}

// Check inspects a database without applying migrations. It will eventually
// compare the live database's schema with the expected latest schema and
// report the bookmark row count. Operational failures are returned as errors;
// schema mismatches belong in SchemaDifferences.
func Check(ctx context.Context, path string) (CheckReport, error) {
	// TODO: open path read-only, inspect version/schema/count, and compare with
	// the expected schema built from the registered migrations.
	return CheckReport{}, ErrCheckNotImplemented
}
