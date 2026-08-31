package i18n

import (
	"strings"
	"testing"
)

func TestDefaultLang(t *testing.T) {
	if CurrentLang() != LangZH {
		t.Fatalf("default lang = %q, want zh", CurrentLang())
	}
}

func TestSetLangAndCurrentLang(t *testing.T) {
	SetLang(LangEN)
	if CurrentLang() != LangEN {
		t.Fatalf("CurrentLang = %q, want en", CurrentLang())
	}
	SetLang(LangZH)
	if CurrentLang() != LangZH {
		t.Fatalf("CurrentLang = %q, want zh", CurrentLang())
	}
}

func TestTChineseAndEnglish(t *testing.T) {
	SetLang(LangZH)
	zh := T("input.placeholder")
	if !strings.Contains(zh, "输入消息") {
		t.Fatalf("zh translation missing Chinese text: %q", zh)
	}

	SetLang(LangEN)
	en := T("input.placeholder")
	if !strings.Contains(en, "Type a message") {
		t.Fatalf("en translation missing English text: %q", en)
	}
}

func TestTFormatting(t *testing.T) {
	SetLang(LangZH)
	if got := T("status.find_results", 3); got != "找到 3 条结果" {
		t.Fatalf("zh formatted = %q, want 找到 3 条结果", got)
	}

	SetLang(LangEN)
	if got := T("status.find_results", 3); got != "Found 3 result(s)" {
		t.Fatalf("en formatted = %q, want Found 3 result(s)", got)
	}
}

func TestTMissingKeyFallback(t *testing.T) {
	SetLang(LangZH)
	if got := T("no.such.key"); got != "no.such.key" {
		t.Fatalf("missing key fallback = %q, want key itself", got)
	}

	SetLang(LangEN)
	if got := T("no.such.key"); got != "no.such.key" {
		t.Fatalf("missing key fallback = %q, want key itself", got)
	}
}

func TestLanguagePackCoverage(t *testing.T) {
	if len(zhMessages) < 30 {
		t.Fatalf("zh language pack has %d entries, want at least 30", len(zhMessages))
	}
	if len(enMessages) < 30 {
		t.Fatalf("en language pack has %d entries, want at least 30", len(enMessages))
	}

	for key := range zhMessages {
		if _, ok := enMessages[key]; !ok {
			t.Errorf("en language pack missing key %q", key)
		}
	}
	for key := range enMessages {
		if _, ok := zhMessages[key]; !ok {
			t.Errorf("zh language pack missing key %q", key)
		}
	}
}
