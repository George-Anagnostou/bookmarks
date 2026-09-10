package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"bookmarks/internal/apiclient"
)

func main() {
	app := newCLI(os.LookupEnv, os.Stdout, os.Stderr, func(cfg apiclient.Config) (bookmarkClient, error) {
		return apiclient.New(cfg)
	})

	err := app.run(context.Background(), os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)

		var usageErr *usageError
		if errors.As(err, &usageErr) {
			usageErr.usage(os.Stderr)
		}

		os.Exit(1)
	}
}
