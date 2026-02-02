package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"negitoro/internal/compiler"
	"negitoro/internal/formatter"
	"negitoro/internal/runtime"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}
	switch os.Args[1] {
	case "build":
		buildCmd(os.Args[2:])
	case "run":
		runCmd(os.Args[2:])
	case "launch":
		launchCmd(os.Args[2:])
	case "format":
		formatCmd(os.Args[2:])
	default:
		usage()
		os.Exit(1)
	}
}

func buildCmd(args []string) {
	fs := flag.NewFlagSet("build", flag.ExitOnError)
	out := fs.String("o", "", "出力ファイルのベース名（入力ファイルと同じフォルダに生成）")
	_ = fs.Parse(args)
	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "入力ファイルが必要です")
		os.Exit(1)
	}
	entry := fs.Arg(0)
	entryDir := filepath.Dir(entry)
	entryBase := filepath.Base(entry)
	if ext := filepath.Ext(entryBase); ext != "" {
		entryBase = entryBase[:len(entryBase)-len(ext)]
	}
	comp := compiler.New()
	res, err := comp.Compile(entry)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	base := *out
	if base == "" {
		base = entryBase
	}
	if filepath.Ext(base) != "" {
		base = base[:len(base)-len(filepath.Ext(base))]
	}
	base = filepath.Base(base)
	basePath := filepath.Join(entryDir, base)
	if err := os.WriteFile(basePath+".wat", []byte(res.Wat), 0644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := os.WriteFile(basePath+".wasm", res.Wasm, 0644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runCmd(args []string) {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	_ = fs.Parse(args)
	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "入力ファイルが必要です")
		os.Exit(1)
	}
	entry := fs.Arg(0)
	// Remaining arguments after the entry file are passed to the script
	scriptArgs := fs.Args()[1:]
	comp := compiler.New()
	res, err := comp.Compile(entry)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	runner := runtime.NewRunner()
	out, err := runner.RunWithArgs(res.Wasm, scriptArgs)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Print(out)
}

func usage() {
	fmt.Fprintln(os.Stderr, "使い方:")
	fmt.Fprintln(os.Stderr, "  negitoro build <entry.ngtr> [-o <name>]")
	fmt.Fprintln(os.Stderr, "  negitoro run <entry.ngtr> [args...]")
	fmt.Fprintln(os.Stderr, "  negitoro launch <entry.wasm> [args...]")
	fmt.Fprintln(os.Stderr, "  negitoro format <file.ngtr> [--write]")
}

func launchCmd(args []string) {
	fs := flag.NewFlagSet("launch", flag.ExitOnError)
	_ = fs.Parse(args)
	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "入力ファイルが必要です")
		os.Exit(1)
	}
	entry := fs.Arg(0)
	scriptArgs := fs.Args()[1:]
	wasm, err := os.ReadFile(entry)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	runner := runtime.NewRunner()
	out, err := runner.RunWithArgs(wasm, scriptArgs)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Print(out)
}

func formatCmd(args []string) {
	fs := flag.NewFlagSet("format", flag.ExitOnError)
	write := fs.Bool("write", false, "ファイルを上書き保存する")
	_ = fs.Parse(args)
	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "入力ファイルが必要です")
		os.Exit(1)
	}

	for _, file := range fs.Args() {
		src, err := os.ReadFile(file)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", file, err)
			os.Exit(1)
		}

		f := formatter.New()
		formatted, err := f.Format(file, string(src))
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", file, err)
			os.Exit(1)
		}

		if *write {
			if err := os.WriteFile(file, []byte(formatted), 0644); err != nil {
				fmt.Fprintf(os.Stderr, "%s: %v\n", file, err)
				os.Exit(1)
			}
		} else {
			fmt.Print(formatted)
		}
	}
}
