// Package main is struct documentation generator
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const filename = "zz_generated.docs.go"

type config struct {
	Path  string
	Tag   string
	All   bool
	Quiet bool
}

func main() {
	os.Exit(realMain(os.Args, os.Stderr))
}

func realMain(args []string, out io.Writer) int {
	var cfg config
	flgs := flag.NewFlagSet("oapi-docgen", flag.ExitOnError)
	flgs.SetOutput(out)
	flgs.StringVar(&cfg.Path, "path", "", "The path to parse for documentation. Defaults to the current working directory.")
	flgs.StringVar(&cfg.Tag, "tag", "json", "The tag to override the documentation key.")
	flgs.BoolVar(&cfg.All, "all", false, "Parse all structs.")
	flgs.BoolVar(&cfg.Quiet, "q", false, "Suppress generation output.")
	flgs.Usage = func() {
		_, _ = fmt.Fprintln(out, "Usage: oapi-docgen [options] schemas")
		_, _ = fmt.Fprintln(out, "Options:")
		flgs.PrintDefaults()
	}
	if err := flgs.Parse(args[1:]); err != nil {
		return 1
	}

	if cfg.Path == "" {
		cwd, err := os.Getwd()
		if err != nil {
			_, _ = fmt.Fprintln(out, "Could not determine current working directory: "+err.Error())
			return 1
		}
		cfg.Path = cwd
	}
	filePath := filepath.Join(cfg.Path, filename)

	if !cfg.Quiet {
		_, _ = fmt.Fprintf(os.Stdout, "Generating Docs for %q\n", cfg.Path)
	}

	// Remove the file if it exists.
	if _, err := os.Stat(filePath); err != nil {
		_ = os.Remove(filePath)
	}

	gen := NewGenerator(cfg.Tag, cfg.All)
	b, err := gen.Generate(cfg.Path)
	if err != nil {
		_, _ = fmt.Fprintf(out, "Could not generate documentation file %q: %s\n", filePath, err.Error())
		return 1
	}
	if b == nil {
		return 0
	}

	//nolint:gosec // The mask 0o644 is fine.
	if err = os.WriteFile(filePath, b, 0o644); err != nil {
		_, _ = fmt.Fprintf(out, "Could not write documentation file %q: %s\n", filePath, err.Error())
		return 1
	}
	return 0
}
