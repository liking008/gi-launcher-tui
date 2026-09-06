# GI Launcher TUI

一个用 **Go + Bubble Tea** 编写的 **原神（Genshin Impact）**（国服/国际服/B服）
终端启动器。参考
[Snap.Hutao.Remastered](https://github.com/SnapHutaoRemasteringProject/Snap.Hutao.Remastered)
与 [an-anime-game-launcher](https://github.com/an-anime-team/an-anime-game-launcher)
的实现，支持多服务器管理、增量/全量下载、断点续传、版本切换与游戏启动。

> Genshin Impact / 原神 launcher for the terminal (Linux/Arch).
>
> **关键词 / keywords**：Genshin Impact · 原神 · 米哈游 · miHoYo · launcher ·
> 启动器 · dwproton · proton · TUI · Bubble Tea · sophon · Linux · Arch

## 功能

- **多服务器**：官服 / B服 / 国际服，切换时通过重命名 `_Data` 目录 + 整理差异文件，
  共享资源只存一次（硬链接），切换零复制。
- **下载 / 更新**：基于官方 sophon 分块接口（`getBuild` → zstd/protobuf 清单 → 分块），
  支持断点续传（按文件跳过）、复用已有资源、热更新文件保留（按大小校验）。
- **增量补丁**：支持旧式 `.hdiff` 增量包（通过 `hpatchz` 应用）。
- **校验**：按 `pkg_version` 清单比对本地文件（MD5 + 大小）。
- **账号切换**：参照胡桃工具箱，把游戏 SDK 写入 wine prefix 注册表（`user.reg`）的
  `MIHOYOSDK_ADL_PROD_*` 自动登录数据保存为账号记录，选择后写回注册表，下次启动即生效。
- **用户协议**：拉取当前服务器协议。
- **游戏启动**：使用系统 `dwproton` + 独立 wine prefix，参考 aagl 的参数
  （`WINEPREFIX`、`WINE_ENABLE_TIMEOUT_FIX=1`、`PROTON_USE_WINED3D=0`、DXVK）。

## 运行环境

仅适用于 **Linux（x86_64/amd64）**，主要面向 **Arch 系发行版**。该项目使用 Linux
特有的 `syscall.SysProcAttr{Setsid}` 等 API，不适用于 Windows / ARM，也不提供
对应产物。

- **启动游戏**依赖系统安装 `dwproton`：Arch / AUR 装 `dwproton-bin`；
  **其他发行版请自行准备 `dwproton`**（可参考
  [an-anime-game-launcher](https://github.com/an-anime-team/an-anime-game-launcher)
  的文档），否则游戏启动功能不可用。
- 下载 / 校验 / 版本切换等**其他功能不依赖 `dwproton`**，没有它也能正常使用。

## 构建与运行

```bash
go build -o gi-launcher .
./gi-launcher        # 在真实终端中运行
```

依赖：Go 1.21+（发布产物为 `CGO_ENABLED=0` 的静态链接可执行文件）。

你也可以从 GitHub Releases 直接下载已构建的静态可执行文件。

## 界面操作

- 主菜单：`↑/↓` 选择 · `enter` 进入 · `q` 退出（仅主页）
- 版本/服务器页：`enter` 确认切换（切换后自动整理）· `t` 整理 · `d` 下载 · `p` 协议
- 下载页：`d` 开始下载 · `t` 整理 · `r` 重新获取
- 账号页：`↑/↓` 选择账号 · `enter` 应用 · `a` 保存当前账号 · `e` 改标签 · `d` 删除
- 设置页：`enter` 编辑路径 · `s` 保存

## 配置

`~/.config/gi-launcher/config.json`（可用 `GIH_LANCHER_CONFIG` 覆盖路径）：
游戏目录、下载目录、版本目录、工具目录、wine prefix、语音包、并发数等。
默认下载/工具目录在 `~/.cache/gi-launcher`，备份在 `<下载目录>/backup`。

## 目录结构

```
internal/
  config/   持久化配置
  game/     游戏目录解析、服务器切换（重命名/整理/备份）、config.ini
  edition/  服务器定义（官服/B服/国际服）
  resource/ 官方 getGamePackages / getGameBranches / 协议 API
  sophon/   sophon 分块下载（zstd + 手写 protobuf 清单、分块组装、复用/跳过）
  download/ 通用断点续传下载器
  manifest/ pkg_version 解析与校验
  patch/    hdiff 增量补丁应用
  archive/  zip 解压
  account/  账号配置与 wine 注册表读写
  tui/      Bubble Tea 界面
```