# 安全扫描与 SBOM

## 安全扫描

- 使用 `govulncheck` 检查 Go 依赖漏洞。
- 本地运行：

```bash
make security-scan
```

- CI 已接入 `Vulnerability scan (govulncheck)` 步骤。

## SBOM（软件物料清单）

- 使用 Go module 信息生成简单 SBOM：

```bash
make sbom
# 输出 dist/sbom.json
```

- Release 时 CI 会自动生成并上传 `agentforge-sbom`。

## 说明

- `govulncheck` 需要联网下载漏洞数据库；离线环境可跳过。
- SBOM 为 Go module 级清单，后续可扩展为 Syft/SPDX 格式。
