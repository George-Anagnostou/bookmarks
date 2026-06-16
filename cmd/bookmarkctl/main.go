package main

import (
	"context"
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
		os.Exit(1)
	}
}
