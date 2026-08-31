package main

import (
	"errors"
	"flag"
	"fmt"
	"github.com/chronosvx/agentforge/internal/config"
	"github.com/chronosvx/agentforge/internal/session"
	"strings"
)

// runExport 实现 `agentforge export --out-dir <dir>`。
func runExport(args []string) error {
	fs := flag.NewFlagSet("agentforge export", flag.ContinueOnError)
	outDir := fs.String("out-dir", "", "导出目录")
	dataDir := fs.String("data-dir", config.DefaultDataDir, "本地数据目录")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("export: unexpected arguments: %v", fs.Args())
	}
	if *outDir == "" {
		return errors.New("export: --out-dir is required")
	}

	dir, err := expandHome(*dataDir)
	if err != nil {
		return err
	}
	out, err := expandHome(*outDir)
	if err != nil {
		return err
	}

	paths, err := session.ExportAll(dir, out)
	if err != nil {
		return err
	}

	fmt.Printf("exported %d session(s) to %s\n", len(paths), out)
	for _, path := range paths {
		fmt.Println(path)
	}
	return nil
}

// runImport 实现 `agentforge import --in-dir <dir>`。
func runImport(args []string) error {
	fs := flag.NewFlagSet("agentforge import", flag.ContinueOnError)
	inDir := fs.String("in-dir", "", "导入目录")
	dataDir := fs.String("data-dir", config.DefaultDataDir, "本地数据目录")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("import: unexpected arguments: %v", fs.Args())
	}
	if *inDir == "" {
		return errors.New("import: --in-dir is required")
	}

	in, err := expandHome(*inDir)
	if err != nil {
		return err
	}
	dir, err := expandHome(*dataDir)
	if err != nil {
		return err
	}

	results, err := session.ImportAll(in, dir)
	if err != nil {
		return err
	}

	imported, skipped := 0, 0
	for _, result := range results {
		fmt.Println(result)
		if strings.HasPrefix(result, "imported ") {
			imported++
		} else {
			skipped++
		}
	}
	fmt.Printf("imported %d, skipped %d\n", imported, skipped)
	return nil
}
