// Command agentforge-plugin-publish 签名并发布一个插件市场条目。
//
// 用法：
//
//	agentforge-plugin-publish <插件名> <版本> <包URL> <私钥PEM路径> <市场索引JSON路径> [描述]
//
// 包 URL 支持 http(s)://、file:// 或本地文件路径；程序会计算其 SHA-256，
// 使用 ECDSA P-256 私钥对规范 JSON 签名，然后追加/更新到市场索引文件。
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/chronosvx/agentforge/internal/plugin"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "agentforge-plugin-publish:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) < 5 || len(args) > 6 {
		return fmt.Errorf("usage: agentforge-plugin-publish <name> <version> <url> <private-key.pem> <index.json> [description]")
	}

	name, version, pkgURL, keyPath, indexPath := args[0], args[1], args[2], args[3], args[4]
	description := name
	if len(args) == 6 {
		description = args[5]
	}

	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		return fmt.Errorf("read private key %q: %w", keyPath, err)
	}

	entry := plugin.MarketIndexEntry{
		Name:        name,
		Version:     version,
		Description: description,
		URL:         pkgURL,
	}
	published, err := plugin.PreparePublish(entry, string(keyPEM))
	if err != nil {
		return err
	}
	if err := plugin.AppendToIndex(indexPath, published); err != nil {
		return err
	}

	out, err := json.MarshalIndent(published, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal published entry: %w", err)
	}
	fmt.Println(string(out))
	return nil
}
