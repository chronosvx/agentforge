package main

import (
	"archive/zip"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// runBackup 实现 `agentforge backup --out backup.zip`。
func runBackup(args []string) error {
	fs := flag.NewFlagSet("backup", flag.ContinueOnError)
	out := fs.String("out", "", "输出备份文件路径（必填）")
	dataDir := fs.String("data-dir", "~/.agentforge", "要备份的数据目录")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*out) == "" {
		return errors.New("backup: --out is required")
	}

	src, err := expandHome(*dataDir)
	if err != nil {
		return err
	}
	dst, err := filepath.Abs(*out)
	if err != nil {
		return err
	}

	if err := zipDir(src, dst); err != nil {
		return fmt.Errorf("backup: %w", err)
	}
	fmt.Printf("backup written to %s\n", dst)
	return nil
}

// runRestore 实现 `agentforge restore --in backup.zip`。
func runRestore(args []string) error {
	fs := flag.NewFlagSet("restore", flag.ContinueOnError)
	in := fs.String("in", "", "备份文件路径（必填）")
	dataDir := fs.String("data-dir", "~/.agentforge", "要恢复到的数据目录")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*in) == "" {
		return errors.New("restore: --in is required")
	}

	src, err := filepath.Abs(*in)
	if err != nil {
		return err
	}
	dst, err := expandHome(*dataDir)
	if err != nil {
		return err
	}

	if err := unzipDir(src, dst); err != nil {
		return fmt.Errorf("restore: %w", err)
	}
	fmt.Printf("restored to %s\n", dst)
	return nil
}

func zipDir(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", src)
	}

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	zw := zip.NewWriter(out)
	defer zw.Close()

	base := filepath.Base(filepath.Clean(src))
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		name := filepath.ToSlash(filepath.Join(base, rel))

		fh, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		fh.Name = name
		fh.Method = zip.Deflate

		w, err := zw.CreateHeader(fh)
		if err != nil {
			return err
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(w, f)
		return err
	})
}

func unzipDir(src, dst string) error {
	zr, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer zr.Close()

	for _, f := range zr.File {
		target := filepath.Join(dst, filepath.FromSlash(f.Name))
		rel, err := filepath.Rel(dst, target)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return fmt.Errorf("unsafe path in archive: %s", f.Name)
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o700); err != nil {
				return err
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
		if err != nil {
			rc.Close()
			return err
		}
		_, copyErr := io.Copy(out, rc)
		rc.Close()
		closeErr := out.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}
