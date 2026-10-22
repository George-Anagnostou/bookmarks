package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"bookmarks/internal/sqlite"
)

// runCheck implements "bookmarkd check DBPATH". It inspects the database
// read-only, writes the report as JSON, and returns an error when the database
// does not match the latest schema this binary supports.
func runCheck(ctx context.Context, args []string, stdout io.Writer) error {
	if len(args) != 1 {
		return errors.New("usage: bookmarkd check DBPATH")
	}
	path := args[0]

	report, err := sqlite.Check(ctx, path)
	if err != nil {
		return fmt.Errorf("check %s: %w", path, err)
	}

	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(report); err != nil {
		return err
	}

	if !report.OK() {
		return fmt.Errorf("check %s: database does not match the expected schema", path)
	}
	return nil
}
