package provider

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// maxImageFileSize 限制单张图片最大 20MB，避免把超大文件直接塞进请求体。
const maxImageFileSize = 20 * 1024 * 1024

// ImagePart 表示一条消息中的一张图片。
// DataURL 是完整的 data:image/...;base64,... URL；MediaType 仅作辅助信息。
type ImagePart struct {
	DataURL   string
	MediaType string
}

// imageMediaTypes 支持通过扩展名推断的图片 MIME 类型。
var imageMediaTypes = map[string]string{
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".webp": "image/webp",
	".gif":  "image/gif",
}

// LoadImageFile 读取本地图片并生成 OpenAI Vision 使用的 data URL。
// 当前支持 png/jpeg/jpg/webp/gif，单文件限制 20MB。
func LoadImageFile(path string) (ImagePart, error) {
	info, err := os.Stat(path)
	if err != nil {
		return ImagePart{}, err
	}
	if info.IsDir() {
		return ImagePart{}, fmt.Errorf("provider: image path is a directory: %s", path)
	}
	if info.Size() > maxImageFileSize {
		return ImagePart{}, fmt.Errorf("provider: image file exceeds 20MB limit: %s", path)
	}

	ext := strings.ToLower(filepath.Ext(path))
	mediaType, ok := imageMediaTypes[ext]
	if !ok {
		return ImagePart{}, fmt.Errorf("provider: unsupported image extension %q (supported: png/jpg/jpeg/webp/gif)", ext)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return ImagePart{}, err
	}
	if len(data) > maxImageFileSize {
		return ImagePart{}, fmt.Errorf("provider: image file exceeds 20MB limit: %s", path)
	}

	return ImagePart{
		DataURL:   "data:" + mediaType + ";base64," + base64.StdEncoding.EncodeToString(data),
		MediaType: mediaType,
	}, nil
}

// LoadImages 批量加载图片；任意一张失败都会返回错误。
func LoadImages(paths []string) ([]ImagePart, error) {
	images := make([]ImagePart, 0, len(paths))
	for _, p := range paths {
		img, err := LoadImageFile(p)
		if err != nil {
			return nil, fmt.Errorf("provider: load image %q: %w", p, err)
		}
		images = append(images, img)
	}
	return images, nil
}

// imageContentPart 是 OpenAI Vision content parts 中的一项。
type imageContentPart struct {
	Type     string         `json:"type"`
	Text     *string        `json:"text,omitempty"`
	ImageURL *imageURLValue `json:"image_url,omitempty"`
}

type imageURLValue struct {
	URL string `json:"url"`
}

// MarshalJSON 在 Message 带图片时把 content 编码为 OpenAI Vision parts 数组；
// 不带图片时保持原有的字符串 content，避免影响普通文本/工具调用消息。
func (m Message) MarshalJSON() ([]byte, error) {
	if len(m.Images) == 0 {
		type plainMessage Message
		return json.Marshal(plainMessage(m))
	}

	parts := make([]imageContentPart, 0, 1+len(m.Images))
	text := m.Content
	parts = append(parts, imageContentPart{Type: "text", Text: &text})
	for _, img := range m.Images {
		parts = append(parts, imageContentPart{
			Type: "image_url",
			ImageURL: &imageURLValue{
				URL: img.DataURL,
			},
		})
	}

	type imageMessage struct {
		Role       string             `json:"role"`
		Content    []imageContentPart `json:"content"`
		ToolCalls  []ToolCall         `json:"tool_calls,omitempty"`
		ToolCallID string             `json:"tool_call_id,omitempty"`
	}
	return json.Marshal(imageMessage{
		Role:       m.Role,
		Content:    parts,
		ToolCalls:  m.ToolCalls,
		ToolCallID: m.ToolCallID,
	})
}

// UnmarshalJSON 兼容 OpenAI 兼容协议中的字符串 content 与 Vision parts 数组 content。
// 字符串直接写入 Content；parts 数组中的 text 拼回 Content，image_url 存入 Images。
func (m *Message) UnmarshalJSON(data []byte) error {
	var raw struct {
		Role       string          `json:"role"`
		Content    json.RawMessage `json:"content"`
		ToolCalls  []ToolCall      `json:"tool_calls"`
		ToolCallID string          `json:"tool_call_id"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	m.Role = raw.Role
	m.ToolCalls = raw.ToolCalls
	m.ToolCallID = raw.ToolCallID
	m.Images = nil

	if len(raw.Content) == 0 || string(raw.Content) == "null" {
		m.Content = ""
		return nil
	}

	if len(raw.Content) > 0 && raw.Content[0] == '"' {
		var s string
		if err := json.Unmarshal(raw.Content, &s); err != nil {
			return err
		}
		m.Content = s
		return nil
	}

	if len(raw.Content) > 0 && raw.Content[0] == '[' {
		var parts []struct {
			Type     string `json:"type"`
			Text     string `json:"text"`
			ImageURL *struct {
				URL string `json:"url"`
			} `json:"image_url"`
		}
		if err := json.Unmarshal(raw.Content, &parts); err != nil {
			return err
		}
		var texts []string
		for _, part := range parts {
			switch strings.ToLower(part.Type) {
			case "text":
				texts = append(texts, part.Text)
			case "image_url":
				if part.ImageURL != nil && part.ImageURL.URL != "" {
					img := ImagePart{DataURL: part.ImageURL.URL}
					if strings.HasPrefix(img.DataURL, "data:") {
						if semi := strings.Index(img.DataURL, ";"); semi > 5 {
							img.MediaType = img.DataURL[len("data:"):semi]
						}
					}
					m.Images = append(m.Images, img)
				}
			}
		}
		m.Content = strings.Join(texts, "")
		return nil
	}

	return fmt.Errorf("provider: unsupported message content type: %s", string(raw.Content))
}
