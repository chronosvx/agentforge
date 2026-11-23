package main

import (
	"errors"
	"flag"
	"fmt"
	"github.com/chronosvx/agentforge/internal/config"
	"github.com/chronosvx/agentforge/internal/session"
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

	report, err := session.ExportAllWithReport(dir, out)
	if err != nil {
		return err
	}

	fmt.Printf("exported %d session(s) to %s (plaintext %d, encrypted %d)\n",
		len(report.Paths), out, report.Plaintext, report.Encrypted)
	for _, path := range report.Paths {
		fmt.Println(path)
	}
	if len(report.Skipped) > 0 {
		fmt.Printf("skipped %d:\n", len(report.Skipped))
		for _, item := range report.Skipped {
			fmt.Println("  " + item)
		}
	}
	if len(report.Failed) > 0 {
		fmt.Printf("failed %d:\n", len(report.Failed))
		for _, item := range report.Failed {
			fmt.Println("  " + item)
		}
		return fmt.Errorf("export: %d session(s) failed", len(report.Failed))
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

	report, err := session.ImportAllWithReport(in, dir)
	if err != nil {
		return err
	}

	for _, item := range report.Imported {
		fmt.Println("imported " + item)
	}
	for _, item := range report.Skipped {
		fmt.Println("skip " + item)
	}
	for _, item := range report.Failed {
		fmt.Println("failed " + item)
	}
	fmt.Printf("imported %d, skipped %d, failed %d\n", len(report.Imported), len(report.Skipped), len(report.Failed))
	if len(report.Failed) > 0 {
		return fmt.Errorf("import: %d item(s) failed", len(report.Failed))
	}
	return nil
}
