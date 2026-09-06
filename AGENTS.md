# AGENTS.md

面向 AI 代理 / 开发者的项目指引。

## 构建与校验

```bash
go build ./...      # 编译
go vet ./...        # 静态检查
gofmt -l .          # 格式检查（必须为空）
go test ./...       # 全部测试（部分在线测试需要网络）
```

提交前必须保证：build / vet / gofmt 全通过，测试全绿。

## 常用命令

```bash
go run .            # 本地运行 TUI（需要真实 TTY）
go test ./internal/sophon/ -run TestGetBuildAndManifestLive -v   # 在线清单测试
```

## 关键约定

- **不要写注释除非必要**：代码保持自解释。
- **包划分**：`internal/` 下的包按职责单一；TUI 只调用上层包，业务逻辑放 `internal/*`。
- **错误处理**：透传底层错误并加上下文（如 `fmt.Errorf("整理失败: %w", err)`）。
- **服务器切换**：`ConvertInstall` 用「重命名 `_Data` 目录 + 移走独有文件」实现最小复制；
  热更新文件必须保留（大小校验而非 MD5），参考 aagl 的 `fast_verify`。

## 测试要点

- 在线测试会访问米哈游 API / CDN（`TestFetchLive`、`TestBranchLive`、`TestProtocolLive`、
  `TestGetBuildAndManifestLive`、`TestDownloadPCSDKLive` 等）。
- 本地测试用 `httptest` 模拟清单/分块，避免网络（如 `reuse_test.go`、`sophon_flow_test.go`）。
- 测试不得写入真实 `~/.config/gi-launcher/config.json`：`testConfig` 已用
  `GIH_LANCHER_CONFIG` 指向临时目录。

## 备份/切换机制

- 备份在 `<下载目录>/backup/<数据目录>/`（按服务器家族分目录）。
- `ConvertInstall`：目标目录不存在则把当前 `_Data` 重命名成目标名；两个都在则把残留
  移入备份。`FixPkgVersion` 重命名后改写根 `pkg_version`。
- `ensurePCSDK`：B服 SDK（`PCGameSDK.dll` + `BLPlatform64`）优先从备份恢复，其次从
  缓存 zip 解压，最后才下载。

## 启动

- 使用系统 `dwproton`（`/usr/share/steam/compatibilitytools.d/dwproton/proton`），
  前缀在 `WinePrefix`，环境变量含 `WINE_ENABLE_TIMEOUT_FIX=1`、`PROTON_USE_WINED3D=0`。
- 启动前会 `killStaleGameProcesses` 清理卡死的 lutris/wine 进程。