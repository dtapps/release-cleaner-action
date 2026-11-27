# release-cleaner（GitHub Action）

[![Release](https://img.shields.io/github/v/release/dtapps/release-cleaner-action?style=flat)](https://github.com/dtapps/release-cleaner-action/releases)
[![Go Version](https://img.shields.io/github/go-mod/go-version/dtapps/release-cleaner-action?style=flat)](https://go.dev/)
[![License](https://img.shields.io/github/license/dtapps/release-cleaner-action?style=flat)](LICENSE)
[![Stars](https://img.shields.io/github/stars/dtapps/release-cleaner-action?style=flat)](https://github.com/dtapps/release-cleaner-action/stargazers)
[![Issues](https://img.shields.io/github/issues/dtapps/release-cleaner-action?style=flat)](https://github.com/dtapps/release-cleaner-action/issues)

自动清理 GitHub 仓库中的旧版本 Release，支持按数量、天数、正则匹配三种模式；开启 `delete_git_tag` 后，还会按相同规则独立清理 Git Tag。参数与 [CNB 版 release-cleaner](https://cnb.cool/dtapp/plugins/release-cleaner) 保持一致，方便在两个平台间迁移。

## 功能特性

- 三种清理模式：按保留数量、按保留天数、按正则匹配
- 保护名单：通过正则表达式保护重要版本不被误删
- 可选独立清理 Git Tag：开启后按相同规则单独筛选，支持只有 Tag 没有 Release 的版本
- 预演模式：先列出要删除的版本，确认无误后再执行
- 失败即失败：API 请求、参数校验或删除失败都会以非零退出码结束工作流
- 超小镜像：基于 Go 静态编译

## 快速开始

### 前置条件

- 工作流所在仓库拥有 Releases 读写权限
- 在 `permissions` 中声明 `contents: write`（删除 Release 与 Git Tag 必需）

### 使用方法

在需要清理版本的工作流中添加：

```yaml
name: Cleanup Releases

on:
  schedule:
    - cron: '0 3 * * 0'
  workflow_dispatch:

permissions:
  contents: write

jobs:
  cleanup:
    runs-on: ubuntu-latest
    steps:
      - name: 清理旧版本
        uses: dtapps/release-cleaner-action@main
        with:
          mode: by_count
          keep: 5
          protect: '^(v\d+\.\d+\.\d+|latest|stable)$'
          dry_run: true
```

### 示例：只清理旧 nightly

下面的配置只把非正式版纳入 `by_count` 清理；`vX.Y.Z`、`latest`、`stable` 以及 `rc` / `beta` / `alpha` 版本会被 `protect` 保护，不会被误删。`dry_run` 必须显式写成 `false` 才会真正删除。

`continue-on-error: true` 表示清理步骤失败只记录日志、不会让整个 Job 失败，后续步骤会继续执行。

```yaml
permissions:
  contents: write

jobs:
  cleanup:
    runs-on: ubuntu-latest
    steps:
      # protect 保住正式版、latest/stable 与 rc/beta/alpha，避免 by_count 误删非 nightly 版本；
      # dry_run 必须显式为 false，否则插件默认只列出待删除版本。
      - name: 清理旧 nightly（只保留最近 3 个）
        uses: dtapps/release-cleaner-action@main
        continue-on-error: true
        with:
          mode: by_count
          keep: 3
          protect: '^(v[0-9]+\.[0-9]+\.[0-9]+(-(rc|beta|alpha)[.-]?[0-9A-Za-z.-]*)?|latest|stable)$'
          delete_git_tag: true
          dry_run: false
```

## 参数说明

> 与 CNB 版参数一一对应，仅将 `settings:` 改为 `with:`。

| 参数 | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `mode` | string | `by_count` | 清理模式：`by_count` / `by_days` / `regex` |
| `keep` | number | `5` | `by_count` 模式下保留最近多少个版本，必须是非负整数 |
| `days` | number | `30` | `by_days` 模式下保留多少天内的版本，必须是非负整数 |
| `regex` | string | `""` | `regex` 模式下匹配要删除的版本名正则，模式为 `regex` 时必填 |
| `protect` | string | 见下方 | 保护名单正则，匹配的版本不会被删除 |
| `delete_git_tag` | boolean | `false` | 是否按相同规则独立清理 Git Tag（不可恢复） |
| `dry_run` | boolean | `true` | 预演模式，只列出不执行删除 |

GitHub 平台额外提供一个 `token` 参数，默认值为 `${{ github.token }}`，一般无需显式传入；如需使用 PAT 或其他 Token，可通过 `with.token` 覆盖。

默认保护正则：

```yaml
with:
  protect: '^(v\d+\.\d+\.\d+|latest|stable)$'
```

## 清理模式详解

### by_count（按数量保留）

保留最近 N 个未受保护的版本，删除更早的版本。适合保持仓库整洁的场景。

```yaml
with:
  mode: by_count
  keep: 5
  protect: '^(v\d+\.\d+\.\d+|latest)$'
```

### by_days（按天数保留）

保留 N 天内的版本，删除更早的版本。适合按时间维度清理。

```yaml
with:
  mode: by_days
  days: 90
```

### regex（正则匹配）

删除所有匹配正则的版本。适合批量清理特定命名的测试版本。

```yaml
with:
  mode: regex
  regex: '^test-'
  protect: '^(v\d+\.\d+\.\d+)$'
```

### Release 与 Git Tag 数量不一致

开启 `delete_git_tag: true` 后，Release 和 Git Tag 会分别获取、分别按相同的 `mode` / `keep` / `days` / `regex` / `protect` 规则筛选并删除，不会按列表下标或数量强行配对。因此，只有 Tag 没有 Release 的版本也会参与清理。

`by_count` 和 `by_days` 模式下，同名 Tag 优先使用 Release 的创建时间；没有对应 Release 的 Tag 使用其指向提交的提交时间。若未受保护的 Tag 无法获取时间，任务会报错停止，避免误删。

## 安全建议

### 首次使用务必开启预演模式

```yaml
with:
  dry_run: true
```

运行后查看日志，确认列出的版本都是你想删除的，再将 `dry_run` 改为 `false`。

### 保护名单要兜底

`protect` 参数至少要包含正式版命名规范，防止误删生产版本：

```yaml
with:
  protect: '^(v\d+\.\d+\.\d+|latest|stable|release-.*)$'
```

### 独立清理 Git Tag

开启 `delete_git_tag` 后，Git Tag 会按相同规则独立清理，包括没有对应 Release 的孤立 Tag。删除 Git Tag 是不可恢复的操作，建议只在清理测试版本时开启：

```yaml
with:
  delete_git_tag: false
```

## 常见问题

### Q: 为什么运行后什么都没删？

可能原因：

1. `dry_run: true`，处于预演模式
2. 所有版本都匹配了 `protect` 保护正则
3. 仓库中没有可清理的 Release 或 Git Tag（未开启 `delete_git_tag` 时只检查 Release）

### Q: 提示 403 Forbidden？

Token 权限不足。请在 Job 或 Workflow 中声明 `permissions: contents: write`，并确认工作流触发的 Token 具备仓库写权限。若仓库受保护分支策略限制，可改用具备写权限的 PAT 并通过 `with.token` 传入。

### Q: 插件失败了会怎样？

参数非法、API 请求失败或删除失败都会输出错误信息并以非零退出码结束，工作流会标记为失败，不会被误判为成功。

### Q: 如何定时清理？

配合 GitHub Actions 的 `schedule` 触发器：

```yaml
on:
  schedule:
    - cron: '0 3 * * 0'

permissions:
  contents: write

jobs:
  cleanup:
    runs-on: ubuntu-latest
    steps:
      - name: 定期清理
        uses: dtapps/release-cleaner-action@main
        with:
          mode: by_days
          days: 30
          dry_run: false
```

## 开发指南

### 本地构建

需要 Go 1.27.4 及以上版本。

```bash
git clone https://github.com/dtapps/release-cleaner-action
cd release-cleaner-action
go build -o cleanup .
```

### 本地测试

Docker 镜像方式：

```bash
docker build -t release-cleaner:test .
docker run --rm \
  -e INPUT_TOKEN=your_token \
  -e GITHUB_REPOSITORY=your_org/your_repo \
  -e INPUT_MODE=by_count \
  -e INPUT_KEEP=5 \
  -e INPUT_DRY_RUN=true \
  release-cleaner:test
```

> GitHub Actions 的 Docker Action 会把 `with:` 中的参数注入为 `INPUT_<参数名大写>` 环境变量，本地直跑时按此规则设置即可。

## 许可证

MIT License
