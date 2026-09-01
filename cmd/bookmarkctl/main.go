package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"bookmarks/internal/apiclient"
)

func main() {
	err := run(
		context.Background(),
		os.Args[1:],
		os.LookupEnv,
		os.Stdout,
		os.Stderr,
		func(cfg apiclient.Config) (bookmarkClient, error) {
			return apiclient.New(cfg)
		},
	)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)

		var usageErr *usageError
		if errors.As(err, &usageErr) {
			usageErr.usage(os.Stderr)
		}

		os.Exit(1)
	}
}
