package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"

	"bookmarks/internal/apiclient"
	"bookmarks/internal/bookmarks"
)

type bookmarkClient interface {
	CreateBookmark(context.Context, bookmarks.CreateInput) (bookmarks.Bookmark, bool, error)
	ListBookmarks(context.Context) ([]bookmarks.Bookmark, error)
}

func run(
	ctx context.Context,
	args []string,
	lookup func(string) (string, bool),
	stdout io.Writer,
	stderr io.Writer,
	newClient func(apiclient.Config) (bookmarkClient, error),
) error {
	if len(args) == 0 {
		return errors.New("command is required")
	}

	switch args[0] {
	case "add":
		return runAdd(ctx, args[1:], lookup, stdout, stderr, newClient)
	case "list":
		return runList(ctx, args[1:], lookup, stdout, stderr, newClient)
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runAdd(
	ctx context.Context,
	args []string,
	lookup func(string) (string, bool),
	stdout io.Writer,
	stderr io.Writer,
	newClient func(apiclient.Config) (bookmarkClient, error),
) error {
	fs := flag.NewFlagSet("add", flag.ContinueOnError)
	fs.SetOutput(stderr)

	title := fs.String("title", "", "bookmark title")
	notes := fs.String("notes", "", "bookmark notes")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("add requires exactly one url")
	}

	cfg, err := loadConfig(lookup)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	client, err := newClient(apiclient.Config{
		BaseURL: cfg.BaseURL,
		Token:   cfg.Token,
	})
	if err != nil {
		return fmt.Errorf("create client: %w", err)
	}

	bookmark, created, err := client.CreateBookmark(ctx, bookmarks.CreateInput{
		URL:    fs.Arg(0),
		Title:  *title,
		Notes:  *notes,
		Source: "bookmarkctl",
	})
	if err != nil {
		return fmt.Errorf("create bookmark: %w", err)
	}

	status := "exists"
	if created {
		status = "created"
	}

	fmt.Fprintf(stdout, "%s %s\n", status, bookmark.URL)
	return nil
}

func runList(
	ctx context.Context,
	args []string,
	lookup func(string) (string, bool),
	stdout io.Writer,
	stderr io.Writer,
	newClient func(apiclient.Config) (bookmarkClient, error),
) error {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	fs.SetOutput(stderr)

	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("list does not take args")
	}

	cfg, err := loadConfig(lookup)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	client, err := newClient(apiclient.Config{
		BaseURL: cfg.BaseURL,
		Token:   cfg.Token,
	})
	if err != nil {
		return fmt.Errorf("create client: %w", err)
	}

	bookmarkList, err := client.ListBookmarks(ctx)
	if err != nil {
		return fmt.Errorf("list bookmarks: %w", err)
	}

	for _, bookmarkItem := range bookmarkList {
		fmt.Fprintf(stdout, "%s\t%s\n", bookmarkItem.URL, bookmarkItem.Title)
	}
	return nil
}
