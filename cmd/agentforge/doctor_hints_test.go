package main

import (
	"errors"
	"testing"
)

func TestAddHintAppends(t *testing.T) {
	var hints []doctorHint
	addHint(&hints, "Go toolchain", "Install Go from https://go.dev/dl")
	addHint(&hints, "GitHub auth (gh auth status)", "运行 gh auth login")

	if len(hints) != 2 {
		t.Fatalf("len(hints) = %d, want 2", len(hints))
	}
	if hints[0].name != "Go toolchain" || hints[0].hint != "Install Go from https://go.dev/dl" {
		t.Fatalf("hints[0] = %+v, want Go toolchain hint", hints[0])
	}
	if hints[1].name != "GitHub auth (gh auth status)" || hints[1].hint != "运行 gh auth login" {
		t.Fatalf("hints[1] = %+v, want GitHub auth hint", hints[1])
	}
}

func TestAddHintSkipsEmpty(t *testing.T) {
	var hints []doctorHint
	addHint(&hints, "Git", "")
	if len(hints) != 0 {
		t.Fatalf("len(hints) = %d, want 0 for empty hint", len(hints))
	}
}

func TestAddHintNilPointerDoesNotPanic(t *testing.T) {
	addHint(nil, "Git", "安装 git")
}

func TestReportWithHintRecordsOnFailure(t *testing.T) {
	var hints []doctorHint
	ok := reportWithHint("Git", errors.New("not found in PATH"), true, &hints, "安装 git")

	if ok {
		t.Fatal("reportWithHint returned true for a failing check")
	}
	if len(hints) != 1 {
		t.Fatalf("len(hints) = %d, want 1", len(hints))
	}
	if hints[0].name != "Git" || hints[0].hint != "安装 git" {
		t.Fatalf("hints[0] = %+v, want Git hint", hints[0])
	}
}

func TestReportWithHintDoesNotRecordOnSuccess(t *testing.T) {
	var hints []doctorHint
	ok := reportWithHint("Go toolchain", nil, false, &hints, "Install Go from https://go.dev/dl")

	if ok {
		t.Fatal("reportWithHint returned true when previous ok was false")
	}
	if len(hints) != 0 {
		t.Fatalf("len(hints) = %d, want 0 for a passing check", len(hints))
	}
}
