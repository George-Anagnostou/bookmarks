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
	UpdateBookmark(context.Context, string, bookmarks.UpdateInput) (bookmarks.Bookmark, error)
	DeleteBookmark(context.Context, string) error
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
	case "edit":
		return runEdit(ctx, args[1:], lookup, stdout, stderr, newClient)
	case "delete":
		return runDelete(ctx, args[1:], lookup, stdout, stderr, newClient)
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

	if len(args) == 0 {
		return errors.New("add requires one url")
	}

	newURL := args[0]

	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("add takes only one url")
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
		URL:    newURL,
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
		fmt.Fprintf(stdout, "%s\t%s\t%s\n", bookmarkItem.ID, bookmarkItem.URL, bookmarkItem.Title)
	}
	return nil
}

func runEdit(
	ctx context.Context,
	args []string,
	lookup func(string) (string, bool),
	stdout io.Writer,
	stderr io.Writer,
	newClient func(apiclient.Config) (bookmarkClient, error),
) error {
	var url optionalStringFlag
	var title optionalStringFlag
	var notes optionalStringFlag
	var source optionalStringFlag

	fs := flag.NewFlagSet("edit", flag.ContinueOnError)
	fs.SetOutput(stderr)

	fs.Var(&url, "url", "bookmark url")
	fs.Var(&title, "title", "bookmark title")
	fs.Var(&notes, "notes", "bookmark notes")
	fs.Var(&source, "source", "bookmark source")

	if len(args) == 0 {
		return errors.New("edit requires an id")
	}

	id := args[0]

	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("edit does not take extra args")
	}

	var input bookmarks.UpdateInput
	if url.set {
		input.URL = &url.value
	}
	if title.set {
		input.Title = &title.value
	}
	if notes.set {
		input.Notes = &notes.value
	}
	if source.set {
		input.Source = &source.value
	}
	if !url.set && !title.set && !notes.set && !source.set {
		return errors.New("edit requires at least one field")
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

	updatedBookmark, err := client.UpdateBookmark(ctx, id, input)
	if err != nil {
		return fmt.Errorf("update bookmark: %w", err)
	}

	fmt.Fprintf(stdout, "updated %s %s\n", updatedBookmark.ID, updatedBookmark.URL)

	return nil
}

func runDelete(
	ctx context.Context,
	args []string,
	lookup func(string) (string, bool),
	stdout io.Writer,
	stderr io.Writer,
	newClient func(apiclient.Config) (bookmarkClient, error),
) error {
	fs := flag.NewFlagSet("delete", flag.ContinueOnError)
	fs.SetOutput(stderr)

	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("delete requires an id")
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

	err = client.DeleteBookmark(ctx, fs.Arg(0))
	if err != nil {
		return fmt.Errorf("delete bookmark: %w", err)
	}

	fmt.Fprintf(stdout, "deleted %s\n", fs.Arg(0))

	return nil
}

type optionalStringFlag struct {
	value string
	set   bool
}

func (f *optionalStringFlag) Set(value string) error {
	f.value = value
	f.set = true
	return nil
}

func (f *optionalStringFlag) String() string {
	return f.value
}
