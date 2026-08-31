package main

import (
	"context"
	"github.com/chronosvx/agentforge/internal/update"
	"net/http"
	"os"
	"time"
)

// maybeCheckUpdate 检查 GitHub 上的新版本，返回可提示的新版本号；没有新版本或检查失败时返回空字符串。
func maybeCheckUpdate() string {
	if os.Getenv("AGENTFORGE_NO_UPDATE_CHECK") != "" {
		return ""
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	latest, err := update.LatestFromGitHubWithClient(ctx, "chronosvx", "agentforge", update.GitHubAPIBase, &http.Client{Timeout: 3 * time.Second})
	if err != nil {
		return ""
	}

	newer, err := update.Check(version, latest)
	if err != nil || !newer {
		return ""
	}
	return latest
}
