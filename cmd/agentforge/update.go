package main

import (
	"context"
	"fmt"
	"github.com/chronosvx/agentforge/internal/update"
	"net/http"
	"os"
	"time"
)

// maybeCheckUpdate 在后台/启动路径静默检查 GitHub 上的新版本。
// 检查失败或网络不可用时忽略错误，不阻塞主流程。
func maybeCheckUpdate() {
	if os.Getenv("AGENTFORGE_NO_UPDATE_CHECK") != "" {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	latest, err := update.LatestFromGitHubWithClient(ctx, "chronosvx", "agentforge", update.GitHubAPIBase, &http.Client{Timeout: 3 * time.Second})
	if err != nil {
		return
	}

	newer, err := update.Check(version, latest)
	if err != nil {
		return
	}
	if newer {
		fmt.Fprintf(os.Stderr, "agentforge: new version %s available (current %s)\n", latest, version)
	}
}
