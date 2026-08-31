package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"github.com/chronosvx/agentforge/internal/config"
	"github.com/chronosvx/agentforge/internal/session"
	"os"
	"os/signal"
	"sort"
	"syscall"
	"time"
)

// runSync 实现 `agentforge sync --from <dir> --to <dir>`。
func runSync(args []string) error {
	fs := flag.NewFlagSet("agentforge sync", flag.ContinueOnError)
	from := fs.String("from", "", "源数据目录")
	to := fs.String("to", "", "目标数据目录")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("sync: unexpected arguments: %v", fs.Args())
	}
	if *from == "" || *to == "" {
		return errors.New("sync: --from and --to are required")
	}

	src, err := expandHome(*from)
	if err != nil {
		return err
	}
	dst, err := expandHome(*to)
	if err != nil {
		return err
	}

	ops, err := session.SyncDir(src, dst)
	if err != nil {
		return err
	}

	ids := make([]string, 0, len(ops))
	for id := range ops {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	fmt.Printf("synced %d session(s)\n", len(ops))
	for _, id := range ids {
		fmt.Printf("%s %s\n", id, ops[id])
	}
	return nil
}

// syncRemoteOptions 保存 `sync-remote` 子命令的解析结果。
type syncRemoteOptions struct {
	baseURL  string
	dataDir  string
	key      string
	push     bool
	pull     bool
	watch    bool
	interval time.Duration
}

// parseSyncRemoteArgs 解析 `sync-remote` 子命令参数。
func parseSyncRemoteArgs(args []string) (syncRemoteOptions, error) {
	fs := flag.NewFlagSet("agentforge sync-remote", flag.ContinueOnError)
	baseURL := fs.String("url", "", "远程存储 base URL，例如 https://host/sessions")
	dataDir := fs.String("data-dir", config.DefaultDataDir, "本地数据目录")
	key := fs.String("key", "", "AES-GCM 同步密钥（hex/base64；为空时读取 DSH_SYNC_KEY）")
	push := fs.Bool("push", false, "上传所有本地会话到远程")
	pull := fs.Bool("pull", false, "下载远程会话到本地")
	watch := fs.Bool("watch", false, "持续同步模式，按 --interval 定时执行")
	interval := fs.Duration("interval", 60*time.Second, "watch 模式的同步间隔")
	if err := fs.Parse(args); err != nil {
		return syncRemoteOptions{}, err
	}
	if fs.NArg() > 0 {
		return syncRemoteOptions{}, fmt.Errorf("sync-remote: unexpected arguments: %v", fs.Args())
	}

	opts := syncRemoteOptions{
		baseURL:  *baseURL,
		dataDir:  *dataDir,
		key:      *key,
		push:     *push,
		pull:     *pull,
		watch:    *watch,
		interval: *interval,
	}
	if opts.baseURL == "" {
		return syncRemoteOptions{}, errors.New("sync-remote: --url is required")
	}
	if !opts.watch && opts.push == opts.pull {
		return syncRemoteOptions{}, errors.New("sync-remote: exactly one of --push or --pull is required")
	}
	if opts.watch && !opts.push && !opts.pull {
		opts.push = true
		opts.pull = true
	}
	if opts.watch && opts.interval <= 0 {
		return syncRemoteOptions{}, errors.New("sync-remote: --interval must be positive")
	}
	return opts, nil
}

// newSyncRemoteStore 根据 --key / DSH_SYNC_KEY 构造普通或加密的 RemoteStore。
func newSyncRemoteStore(baseURL, key string) (session.RemoteStore, error) {
	var remote session.RemoteStore = session.NewHTTPStore(baseURL)
	if key != "" || os.Getenv("DSH_SYNC_KEY") != "" {
		enc, err := session.NewRemoteEncryptedStore(remote, key)
		if err != nil {
			return nil, err
		}
		remote = enc
	}
	return remote, nil
}

// syncOnce 执行一次远程同步。pull 在前、push 在后，便于 watch 默认“先拉后推”。
func syncOnce(ctx context.Context, dataDir string, remote session.RemoteStore, push, pull bool) error {
	if pull {
		ids, err := session.SyncFromRemote(ctx, dataDir, remote)
		if err != nil {
			return err
		}
		fmt.Printf("downloaded %d session(s)\n", len(ids))
		for _, id := range ids {
			fmt.Println(id)
		}
	}
	if push {
		ids, err := session.SyncToRemote(ctx, dataDir, remote)
		if err != nil {
			return err
		}
		fmt.Printf("uploaded %d session(s)\n", len(ids))
		for _, id := range ids {
			fmt.Println(id)
		}
	}
	return nil
}

// runSyncRemote 实现 `agentforge sync-remote --url <base> --data-dir <dir> [--key <hex>] [--push|--pull] [--watch] [--interval <dur>]`。
func runSyncRemote(args []string) error {
	opts, err := parseSyncRemoteArgs(args)
	if err != nil {
		return err
	}

	dir, err := expandHome(opts.dataDir)
	if err != nil {
		return err
	}

	remote, err := newSyncRemoteStore(opts.baseURL, opts.key)
	if err != nil {
		return err
	}

	if !opts.watch {
		return syncOnce(context.Background(), dir, remote, opts.push, opts.pull)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	fmt.Printf("sync-remote: watching %s every %s (push=%v pull=%v); press Ctrl+C to stop\n", opts.baseURL, opts.interval, opts.push, opts.pull)
	for {
		if err := syncOnce(ctx, dir, remote, opts.push, opts.pull); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			fmt.Fprintln(os.Stderr, "agentforge: sync-remote:", err)
		}

		select {
		case <-ctx.Done():
			fmt.Println("sync-remote: stopped")
			return nil
		case <-time.After(opts.interval):
		}
	}
}
