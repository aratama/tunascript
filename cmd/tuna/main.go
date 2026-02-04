package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"tuna/internal/compiler"
	"tuna/internal/formatter"
	"tuna/internal/parser"
	"tuna/internal/runtime"
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
	sandbox := fs.Bool("sandbox", false, "サンドボックスモードで実行する")
	_ = fs.Parse(args)
	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "入力ファイルが必要です")
		os.Exit(1)
	}
	entry := fs.Arg(0)
	// Remaining arguments after the entry file are passed to the script
	scriptArgs := fs.Args()[1:]
	if *sandbox {
		runSandbox(entry, scriptArgs)
		return
	}
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

func runSandbox(entry string, scriptArgs []string) {
	result := runtime.SandboxResult{ExitCode: 0}
	comp := compiler.New()
	res, err := comp.Compile(entry)
	if err != nil {
		result.ExitCode = 1
		result.Error = err.Error()
		printSandboxJSON(result)
		return
	}
	runner := runtime.NewRunner()
	result = runner.RunSandboxWithArgs(res.Wasm, scriptArgs)
	printSandboxJSON(result)
}

func printSandboxJSON(result runtime.SandboxResult) {
	data, err := json.Marshal(result)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(string(data))
}

func usage() {
	fmt.Fprintln(os.Stderr, "使い方:")
	fmt.Fprintln(os.Stderr, "  tuna build <entry.tuna> [-o <name>]")
	fmt.Fprintln(os.Stderr, "  tuna run [--sandbox] <entry.tuna> [args...]")
	fmt.Fprintln(os.Stderr, "  tuna launch <entry.wasm> [args...]")
	fmt.Fprintln(os.Stderr, "  tuna format <file.tuna> [--write]")
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
	annotate := fs.Bool("type", false, "型推論で決定した型注釈を追加する")
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
		var formatted string
		if *annotate {
			p := parser.New(file, string(src))
			mod, err := p.ParseModule()
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: %v\n", file, err)
				os.Exit(1)
			}
			if err := f.AnnotateModuleTypes(mod); err != nil {
				fmt.Fprintf(os.Stderr, "%s: %v\n", file, err)
				os.Exit(1)
			}
			formatted = f.FormatModule(mod)
		} else {
			formatted, err = f.Format(file, string(src))
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: %v\n", file, err)
				os.Exit(1)
			}
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
