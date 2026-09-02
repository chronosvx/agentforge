package main

import (
	"flag"
	"fmt"

	"github.com/chronosvx/agentforge/internal/config"
)

// runConfig 实现 `agentforge config`：显示当前生效配置（隐藏敏感字段）。
func runConfig(args []string) error {
	fs := flag.NewFlagSet("config", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Parse(nil)
	if err != nil {
		return err
	}

	fmt.Println("agentforge config")
	fmt.Println("================")
	fmt.Printf("data-dir:          %s\n", cfg.DataDir)
	fmt.Printf("lang:              %s\n", cfg.Lang)
	fmt.Printf("mock:              %v\n", cfg.Mock)
	fmt.Printf("provider:          %s\n", cfg.Provider)
	fmt.Printf("model:             %s\n", cfg.Model)
	fmt.Printf("provider-base-url: %s\n", cfg.ProviderBaseURL)
	fmt.Printf("api-url:           %s\n", cfg.APIURL)
	fmt.Printf("keychain:          %v\n", cfg.Keychain)
	fmt.Printf("encrypt-sessions:  %v\n", cfg.EncryptSessions)
	fmt.Printf("telemetry:         %v\n", cfg.Telemetry)
	fmt.Printf("plugins-enabled:   %v\n", cfg.PluginsEnabled)
	fmt.Printf("no-color:          %v\n", cfg.NoColor)
	fmt.Printf("provider-key:      %s\n", maskSecret(cfg.ProviderKey))
	fmt.Printf("api-token:         %s\n", maskSecret(cfg.APIToken))
	fmt.Printf("secret-key:        %s\n", maskSecret(cfg.SecretKey))
	return nil
}

func maskSecret(s string) string {
	if s == "" {
		return "(not set)"
	}
	if len(s) <= 4 {
		return "****"
	}
	return s[:2] + "****" + s[len(s)-2:]
}
