package main

import (
	"errors"
	"flag"
	"fmt"
	"github.com/chronosvx/agentforge/internal/config"
	"github.com/chronosvx/agentforge/internal/security"
	"github.com/chronosvx/agentforge/internal/session"
	"github.com/chronosvx/agentforge/internal/share"
	"os"
)

// runShare 实现 `agentforge share --session <id> [--key <key>]`。
func runShare(args []string) error {
	fs := flag.NewFlagSet("agentforge share", flag.ContinueOnError)
	sessionID := fs.String("session", "", "要分享的会话 ID")
	key := fs.String("key", "", "AES-GCM 分享密钥（hex/base64；为空时读取 DSH_SHARE_KEY）")
	shareKey := fs.String("share-key", "", "AES-GCM 分享密钥（--key 的别名）")
	dataDir := fs.String("data-dir", config.DefaultDataDir, "本地数据目录")
	noAudit := fs.Bool("no-audit", false, "禁用审计日志（默认开启）")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("share: unexpected arguments: %v", fs.Args())
	}
	if *sessionID == "" {
		return errors.New("share: --session is required")
	}
	if *key != "" && *shareKey != "" && *key != *shareKey {
		return errors.New("share: --key and --share-key conflict")
	}
	if *key == "" {
		*key = *shareKey
	}

	dir, err := expandHome(*dataDir)
	if err != nil {
		return err
	}
	store, err := session.NewStore(dir)
	if err != nil {
		return err
	}
	entry, err := store.Load(*sessionID)
	if err != nil {
		return fmt.Errorf("share: load session %s: %w", *sessionID, err)
	}

	secretKey, err := resolveShareKey(*key)
	if err != nil {
		return err
	}
	link, err := share.ShareSession(*entry, secretKey)
	if err != nil {
		return err
	}
	audit(newAuditLogger(dir, !*noAudit), "session.share", map[string]any{"sessionId": *sessionID})
	fmt.Println(link)
	return nil
}

// runOpen 实现 `agentforge open --url <dsh-share://...> [--key <key>]`。
func runOpen(args []string) error {
	fs := flag.NewFlagSet("agentforge open", flag.ContinueOnError)
	shareURL := fs.String("url", "", "分享链接，例如 dsh-share://...")
	key := fs.String("key", "", "AES-GCM 分享密钥（hex/base64；为空时读取 DSH_SHARE_KEY）")
	shareKey := fs.String("share-key", "", "AES-GCM 分享密钥（--key 的别名）")
	dataDir := fs.String("data-dir", config.DefaultDataDir, "本地数据目录")
	noAudit := fs.Bool("no-audit", false, "禁用审计日志（默认开启）")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("open: unexpected arguments: %v", fs.Args())
	}
	if *shareURL == "" {
		return errors.New("open: --url is required")
	}
	if *key != "" && *shareKey != "" && *key != *shareKey {
		return errors.New("open: --key and --share-key conflict")
	}
	if *key == "" {
		*key = *shareKey
	}

	dir, err := expandHome(*dataDir)
	if err != nil {
		return err
	}
	secretKey, err := resolveShareKey(*key)
	if err != nil {
		return err
	}
	entry, err := share.OpenShare(*shareURL, secretKey)
	if err != nil {
		return err
	}
	if entry.ID == "" {
		return errors.New("open: shared session has empty id")
	}

	store, err := session.NewStore(dir)
	if err != nil {
		return err
	}
	if _, err := store.Load(entry.ID); err == nil {
		fmt.Printf("skipped %s: already exists locally\n", entry.ID)
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("open: load existing %s: %w", entry.ID, err)
	}

	if err := store.Save(*entry); err != nil {
		return fmt.Errorf("open: save %s: %w", entry.ID, err)
	}
	audit(newAuditLogger(dir, !*noAudit), "session.import", map[string]any{"sessionId": entry.ID})
	fmt.Printf("imported %s\n", entry.ID)
	return nil
}

// resolveShareKey 解析分享密钥；未提供 key 且环境变量为空时返回 nil（明文模式）。
func resolveShareKey(key string) ([]byte, error) {
	if key == "" {
		key = os.Getenv(share.EnvKey)
	}
	if key == "" {
		return nil, nil
	}
	secretKey, err := security.LoadKey(key)
	if err != nil {
		return nil, fmt.Errorf("share: load key: %w", err)
	}
	return secretKey, nil
}
