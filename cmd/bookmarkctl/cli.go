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

const version = "dev"

type bookmarkClient interface {
	CreateBookmark(context.Context, bookmarks.CreateInput) (bookmarks.Bookmark, bool, error)
	ListBookmarks(context.Context, bookmarks.ListQuery) ([]bookmarks.Bookmark, error)
	UpdateBookmark(context.Context, string, bookmarks.UpdateInput) (bookmarks.Bookmark, error)
	DeleteBookmark(context.Context, string) error
}

type cli struct {
	lookup    func(string) (string, bool)
	stdout    io.Writer
	stderr    io.Writer
	newClient func(apiclient.Config) (bookmarkClient, error)
}

func newCLI(lookup func(string) (string, bool), stdout, stderr io.Writer, client func(cfg apiclient.Config) (bookmarkClient, error)) *cli {
	return &cli{
		lookup,
		stdout,
		stderr,
		client,
	}
}

func (app *cli) run(ctx context.Context, args []string) error {
	root := flag.NewFlagSet("bookmarkctl", flag.ContinueOnError)
	root.SetOutput(app.stderr)

	help := root.Bool("h", false, "show help")
	root.BoolVar(help, "help", false, "show help")

	versionRequested := root.Bool("v", false, "show version")
	root.BoolVar(versionRequested, "version", false, "show version")

	root.Usage = func() {
		usage := `Usage: bookmarkctl <command> [flags]

Commands:
	add			Save a bookmark
	list		List bookmarks
	edit		Update a bookmark
	delete		Delete a bookmark

Flags:`
		fmt.Fprintln(root.Output(), usage)
		root.PrintDefaults()
	}

	if err := root.Parse(args); err != nil {
		return newUsageError(err, root)
	}

	if *help {
		root.SetOutput(app.stdout)
		root.Usage()
		root.SetOutput(app.stderr)
		return nil
	}

	if *versionRequested {
		fmt.Fprintln(app.stdout, version)
		return nil
	}

	if root.NArg() == 0 {
		return newUsageError(errors.New("command is required"), root)
	}

	command := root.Arg(0)
	commandArgs := root.Args()[1:]

	switch command {
	case "add":
		return app.runAdd(ctx, commandArgs)
	case "list":
		return app.runList(ctx, commandArgs)
	case "edit":
		return app.runEdit(ctx, commandArgs)
	case "delete":
		return app.runDelete(ctx, commandArgs)
	default:
		return fmt.Errorf("unknown command %q", command)
	}
}

func (app *cli) runAdd(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("add", flag.ContinueOnError)
	fs.SetOutput(app.stderr)

	help := fs.Bool("help", false, "show help")
	fs.BoolVar(help, "h", false, "show help")

	title := fs.String("title", "", "bookmark title")
	notes := fs.String("notes", "", "bookmark notes")

	fs.Usage = func() {
		usage := "Usage: bookmarkctl add [flags] url"
		fmt.Fprintln(fs.Output(), usage)
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		return newUsageError(err, fs)
	}

	if *help {
		fs.SetOutput(app.stdout)
		fs.Usage()
		fs.SetOutput(app.stderr)
		return nil
	}

	if fs.NArg() != 1 {
		return newUsageError(errors.New("exactly one url is required"), fs)
	}

	newURL := fs.Arg(0)

	client, err := app.client()
	if err != nil {
		return err
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
		status = "added"
	}

	fmt.Fprintf(app.stdout, "%s %s\n", status, bookmark.URL)
	return nil
}

func (app *cli) runList(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	fs.SetOutput(app.stderr)
	help := fs.Bool("help", false, "show help")
	fs.BoolVar(help, "h", false, "show help")
	long := false

	fs.BoolVar(&long, "l", false, "show all table fields")
	fs.BoolVar(&long, "long", false, "show all table fields")

	query := fs.String("query", "", "search term")
	limit := fs.Int("limit", 0, "limit")
	offset := fs.Int("offset", 0, "offset")
	format := fs.String("format", "", "output format: table, tsv, json")
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "Usage: bookmarkctl list [flags]")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		return newUsageError(err, fs)
	}

	if *help {
		writeUsage(app.stdout, fs)
		return nil
	}

	if fs.NArg() != 0 {
		return newUsageError(errors.New("list does not take positional arguments"), fs)
	}

	if *limit < 0 {
		return fmt.Errorf("limit must be non-negative")
	}

	if *offset < 0 {
		return fmt.Errorf("offset must be non-negative")
	}

	listFormat, err := ResolveListFormat(*format, app.stdout, isTerminal)
	if err != nil {
		return err
	}

	width := 0
	if listFormat == ListFormatTable && isTerminal(app.stdout) {
		width, err = terminalWidth(app.stdout)
		if err != nil {
			return fmt.Errorf("could not get terminal width: %w", err)
		}
	}

	if long && listFormat != ListFormatTable {
		return fmt.Errorf("-l is only valid with table output")
	}

	client, err := app.client()
	if err != nil {
		return err
	}

	listQuery := bookmarks.ListQuery{
		Query:  *query,
		Limit:  *limit,
		Offset: *offset,
	}

	listFormatOptions := ListFormatOptions{
		Format: listFormat,
		Long:   long,
		Width:  width,
	}

	bookmarkList, err := client.ListBookmarks(ctx, listQuery)
	if err != nil {
		return fmt.Errorf("list bookmarks: %w", err)
	}

	return WriteListBookmarks(app.stdout, bookmarkList, listFormatOptions)
}

func (app *cli) runEdit(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("edit", flag.ContinueOnError)
	fs.SetOutput(app.stderr)
	help := fs.Bool("help", false, "show help")
	fs.BoolVar(help, "h", false, "show help")
	var url optionalStringFlag
	var title optionalStringFlag
	var notes optionalStringFlag
	var source optionalStringFlag

	fs.Var(&url, "url", "bookmark url")
	fs.Var(&title, "title", "bookmark title")
	fs.Var(&notes, "notes", "bookmark notes")
	fs.Var(&source, "source", "bookmark source")
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "Usage: bookmarkctl edit [flags] id")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		return newUsageError(err, fs)
	}

	if *help {
		writeUsage(app.stdout, fs)
		return nil
	}

	if fs.NArg() != 1 {
		return newUsageError(errors.New("exactly one id is required"), fs)
	}

	id := fs.Arg(0)

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

	client, err := app.client()
	if err != nil {
		return err
	}

	updatedBookmark, err := client.UpdateBookmark(ctx, id, input)
	if err != nil {
		return fmt.Errorf("update bookmark: %w", err)
	}

	fmt.Fprintf(app.stdout, "updated %s\n", updatedBookmark.ID)

	return nil
}

func (app *cli) runDelete(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("delete", flag.ContinueOnError)
	fs.SetOutput(app.stderr)
	help := fs.Bool("help", false, "show help")
	fs.BoolVar(help, "h", false, "show help")
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "Usage: bookmarkctl delete id")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		return newUsageError(err, fs)
	}

	if *help {
		writeUsage(app.stdout, fs)
		return nil
	}

	if fs.NArg() != 1 {
		return newUsageError(errors.New("exactly one id is required"), fs)
	}

	id := fs.Arg(0)

	client, err := app.client()
	if err != nil {
		return err
	}

	err = client.DeleteBookmark(ctx, id)
	if err != nil {
		return fmt.Errorf("delete bookmark: %w", err)
	}

	fmt.Fprintf(app.stdout, "deleted %s\n", id)

	return nil
}

func (app *cli) client() (bookmarkClient, error) {
	cfg, err := loadConfig(app.lookup)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	client, err := app.newClient(apiclient.Config{
		BaseURL: cfg.BaseURL,
		Token:   cfg.Token,
	})
	if err != nil {
		return nil, fmt.Errorf("create client: %w", err)
	}

	return client, nil
}

func writeUsage(w io.Writer, fs *flag.FlagSet) {
	previous := fs.Output()
	fs.SetOutput(w)
	fs.Usage()
	fs.SetOutput(previous)
}

type usageError struct {
	err   error
	usage func(io.Writer)
}

func newUsageError(err error, fs *flag.FlagSet) *usageError {
	return &usageError{
		err: err,
		usage: func(w io.Writer) {
			writeUsage(w, fs)
		},
	}
}

func (e *usageError) Error() string {
	return e.err.Error()
}

func (e *usageError) Unwrap() error {
	return e.err
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
