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

func newCommandFlagSet(name string) (*flag.FlagSet, *bool) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	help := fs.Bool("h", false, "show help")
	fs.BoolVar(help, "help", false, "show help")

	return fs, help
}

func newRootFlagSet() (*flag.FlagSet, *bool, *bool) {
	fs, help := newCommandFlagSet("bookmarkctl")
	versionRequested := fs.Bool("v", false, "show version")
	fs.BoolVar(versionRequested, "version", false, "show version")
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), `Usage: bookmarkctl <command> [options]

Commands:
  add     Save a bookmark
  list    List bookmarks
  edit    Update a bookmark
  delete  Delete a bookmark
  help    Show help for a command

Options:`)
		fs.PrintDefaults()
	}

	return fs, help, versionRequested
}

func writeUsage(w io.Writer, fs *flag.FlagSet) {
	fs.SetOutput(w)
	fs.Usage()
	fs.SetOutput(io.Discard)
}

type cliArgs struct {
	command string
	args    []string
}

func parseRootArgs(args []string, stdout, stderr io.Writer) (*cliArgs, error) {
	fs, help, versionRequested := newRootFlagSet()

	if err := fs.Parse(args); err != nil {
		return nil, newUsageError(err, fs)
	}

	if *help {
		writeUsage(stdout, fs)
		return nil, nil
	}

	if *versionRequested {
		fmt.Fprintln(stdout, version)
		return nil, nil
	}

	if fs.NArg() == 0 {
		return nil, newUsageError(errors.New("command is required"), fs)
	}

	command := fs.Arg(0)
	return &cliArgs{
		command: command,
		args:    fs.Args()[1:],
	}, nil
}

func run(
	ctx context.Context,
	args []string,
	lookup func(string) (string, bool),
	stdout io.Writer,
	stderr io.Writer,
	newClient func(apiclient.Config) (bookmarkClient, error),
) error {
	cliArgs, err := parseRootArgs(args, stdout, stderr)
	if err != nil {
		return err
	}

	if cliArgs == nil {
		return nil
	}

	switch cliArgs.command {
	case "add":
		return runAdd(ctx, cliArgs.args, lookup, stdout, stderr, newClient)
	case "list":
		return runList(ctx, cliArgs.args, lookup, stdout, stderr, newClient)
	case "edit":
		return runEdit(ctx, cliArgs.args, lookup, stdout, stderr, newClient)
	case "delete":
		return runDelete(ctx, cliArgs.args, lookup, stdout, stderr, newClient)
	case "help":
		return runHelp(ctx, cliArgs.args, lookup, stdout, stderr, newClient)
	default:
		return fmt.Errorf("unknown command %q", cliArgs.command)
	}
}

func runHelp(
	ctx context.Context,
	args []string,
	lookup func(string) (string, bool),
	stdout io.Writer,
	stderr io.Writer,
	newClient func(apiclient.Config) (bookmarkClient, error),
) error {
	if len(args) == 0 {
		fs, _, _ := newRootFlagSet()
		writeUsage(stdout, fs)
		return nil
	}
	if len(args) != 1 {
		fs, _, _ := newRootFlagSet()
		return newUsageError(errors.New("help accepts at most one command"), fs)
	}

	switch args[0] {
	case "add":
		return runAdd(ctx, []string{"--help"}, lookup, stdout, stderr, newClient)
	case "list":
		return runList(ctx, []string{"--help"}, lookup, stdout, stderr, newClient)
	case "edit":
		return runEdit(ctx, []string{"--help"}, lookup, stdout, stderr, newClient)
	case "delete":
		return runDelete(ctx, []string{"--help"}, lookup, stdout, stderr, newClient)
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
	fs, help := newCommandFlagSet("add")
	title := fs.String("title", "", "bookmark title")
	notes := fs.String("notes", "", "bookmark notes")

	if err := fs.Parse(args); err != nil {
		return newUsageError(err, fs)
	}

	if *help {
		writeUsage(stdout, fs)
		return nil
	}

	if fs.NArg() != 1 {
		return newUsageError(errors.New("exactly one url is required"), fs)
	}

	newURL := fs.Arg(0)

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
	fs, help := newCommandFlagSet("list")
	long := false

	fs.BoolVar(&long, "l", false, "show all table fields")
	fs.BoolVar(&long, "long", false, "show all table fields")

	query := fs.String("query", "", "search term")
	limit := fs.Int("limit", 0, "limit")
	offset := fs.Int("offset", 0, "offset")
	output := fs.String("output", "", "output format: table, tsv, json")

	if err := fs.Parse(args); err != nil {
		return newUsageError(err, fs)
	}

	if *help {
		writeUsage(stdout, fs)
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

	format, err := ResolveListFormat(*output, stdout, isTerminal)
	if err != nil {
		return err
	}

	width := 0
	if format == ListFormatTable && isTerminal(stdout) {
		width, err = terminalWidth(stdout)
		if err != nil {
			return fmt.Errorf("could not get terminal width: %w", err)
		}
	}

	if long && format != ListFormatTable {
		return fmt.Errorf("-l is only valid with table output")
	}

	cfg, err := loadConfig(lookup)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	listQuery := bookmarks.ListQuery{
		Query:  *query,
		Limit:  *limit,
		Offset: *offset,
	}

	listFormatOptions := ListFormatOptions{
		Format: format,
		Long:   long,
		Width:  width,
	}

	client, err := newClient(apiclient.Config{
		BaseURL: cfg.BaseURL,
		Token:   cfg.Token,
	})
	if err != nil {
		return fmt.Errorf("create client: %w", err)
	}

	bookmarkList, err := client.ListBookmarks(ctx, listQuery)
	if err != nil {
		return fmt.Errorf("list bookmarks: %w", err)
	}

	return WriteListBookmarks(stdout, bookmarkList, listFormatOptions)
}

func runEdit(
	ctx context.Context,
	args []string,
	lookup func(string) (string, bool),
	stdout io.Writer,
	stderr io.Writer,
	newClient func(apiclient.Config) (bookmarkClient, error),
) error {
	fs, help := newCommandFlagSet("edit")
	var url optionalStringFlag
	var title optionalStringFlag
	var notes optionalStringFlag
	var source optionalStringFlag

	fs.Var(&url, "url", "bookmark url")
	fs.Var(&title, "title", "bookmark title")
	fs.Var(&notes, "notes", "bookmark notes")
	fs.Var(&source, "source", "bookmark source")

	if err := fs.Parse(args); err != nil {
		return newUsageError(err, fs)
	}

	if *help {
		writeUsage(stdout, fs)
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
	fs, help := newCommandFlagSet("delete")

	if err := fs.Parse(args); err != nil {
		return newUsageError(err, fs)
	}

	if *help {
		writeUsage(stdout, fs)
		return nil
	}

	if fs.NArg() != 1 {
		return newUsageError(errors.New("exactly one id is required"), fs)
	}

	id := fs.Arg(0)

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

	err = client.DeleteBookmark(ctx, id)
	if err != nil {
		return fmt.Errorf("delete bookmark: %w", err)
	}

	fmt.Fprintf(stdout, "deleted %s\n", id)

	return nil
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
