package provider

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadImageFileGeneratesDataURL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.png")
	data := []byte("fake-png-content")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	img, err := LoadImageFile(path)
	if err != nil {
		t.Fatalf("LoadImageFile() error: %v", err)
	}
	wantDataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(data)
	if img.DataURL != wantDataURL {
		t.Fatalf("DataURL = %q, want %q", img.DataURL, wantDataURL)
	}
	if img.MediaType != "image/png" {
		t.Fatalf("MediaType = %q, want image/png", img.MediaType)
	}
}

func TestLoadImageFileRejectsUnsupportedExtension(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o600); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}
	if _, err := LoadImageFile(path); err == nil {
		t.Fatal("LoadImageFile() error = nil, want unsupported extension error")
	}
}

func TestMessageMarshalJSONNoImagesKeepsStringContent(t *testing.T) {
	msg := Message{
		Role:    "user",
		Content: "hello",
	}
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal() error: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal raw error: %v", err)
	}
	if raw["role"] != "user" {
		t.Fatalf("role = %#v, want user", raw["role"])
	}
	if raw["content"] != "hello" {
		t.Fatalf("content = %#v, want string hello", raw["content"])
	}
	if _, ok := raw["content"].(string); !ok {
		t.Fatalf("content type = %T, want string", raw["content"])
	}
}

func TestMessageMarshalJSONWithImagesUsesPartsArray(t *testing.T) {
	msg := Message{
		Role:    "user",
		Content: "what is this?",
		Images: []ImagePart{
			{DataURL: "data:image/png;base64,AAAA", MediaType: "image/png"},
			{DataURL: "data:image/jpeg;base64,BBBB", MediaType: "image/jpeg"},
		},
	}
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal() error: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal raw error: %v", err)
	}
	if raw["role"] != "user" {
		t.Fatalf("role = %#v, want user", raw["role"])
	}
	parts, ok := raw["content"].([]any)
	if !ok {
		t.Fatalf("content type = %T, want []any", raw["content"])
	}
	if len(parts) != 3 {
		t.Fatalf("len(content parts) = %d, want 3; parts=%#v", len(parts), parts)
	}

	textPart, ok := parts[0].(map[string]any)
	if !ok || textPart["type"] != "text" || textPart["text"] != "what is this?" {
		t.Fatalf("first content part = %#v, want text part", parts[0])
	}
	for i, wantURL := range []string{"data:image/png;base64,AAAA", "data:image/jpeg;base64,BBBB"} {
		part, ok := parts[i+1].(map[string]any)
		if !ok || part["type"] != "image_url" {
			t.Fatalf("content part %d = %#v, want image_url", i+1, parts[i+1])
		}
		img, ok := part["image_url"].(map[string]any)
		if !ok || img["url"] != wantURL {
			t.Fatalf("image_url part %d = %#v, want url %q", i, part["image_url"], wantURL)
		}
		if _, hasText := part["text"]; hasText {
			t.Fatalf("image_url part %d unexpectedly contains text field: %#v", i+1, part)
		}
	}
}

func TestToOpenAIMessagesIncludesImageParts(t *testing.T) {
	got := ToOpenAIMessages([]Message{
		{
			Role:    "user",
			Content: "describe",
			Images:  []ImagePart{{DataURL: "data:image/png;base64,AAAA", MediaType: "image/png"}},
		},
	})
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	parts, ok := got[0]["content"].([]map[string]any)
	if !ok {
		t.Fatalf("content type = %T, want []map[string]any", got[0]["content"])
	}
	if len(parts) != 2 {
		t.Fatalf("len(parts) = %d, want 2", len(parts))
	}
	if parts[0]["type"] != "text" || parts[0]["text"] != "describe" {
		t.Fatalf("parts[0] = %#v, want text part", parts[0])
	}
	if parts[1]["type"] != "image_url" {
		t.Fatalf("parts[1] = %#v, want image_url part", parts[1])
	}
}

func TestMessageUnmarshalJSONStringContent(t *testing.T) {
	var msg Message
	if err := json.Unmarshal([]byte(`{"role":"user","content":"hello"}`), &msg); err != nil {
		t.Fatalf("Unmarshal() error: %v", err)
	}
	if msg.Role != "user" || msg.Content != "hello" {
		t.Fatalf("message = %#v, want role user content hello", msg)
	}
	if len(msg.Images) != 0 {
		t.Fatalf("Images = %#v, want empty", msg.Images)
	}
}

func TestMessageUnmarshalJSONPartsContent(t *testing.T) {
	data := []byte(`{
		"role": "user",
		"content": [
			{"type": "text", "text": "see "},
			{"type": "image_url", "image_url": {"url": "data:image/png;base64,AAAA"}},
			{"type": "text", "text": "this"}
		]
	}`)
	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		t.Fatalf("Unmarshal() error: %v", err)
	}
	if msg.Role != "user" || msg.Content != "see this" {
		t.Fatalf("message = %#v, want content %q", msg, "see this")
	}
	if len(msg.Images) != 1 {
		t.Fatalf("Images = %#v, want one image", msg.Images)
	}
	if msg.Images[0].DataURL != "data:image/png;base64,AAAA" {
		t.Fatalf("Images[0].DataURL = %q, want data URL", msg.Images[0].DataURL)
	}
	if msg.Images[0].MediaType != "image/png" {
		t.Fatalf("Images[0].MediaType = %q, want image/png", msg.Images[0].MediaType)
	}
}
