skilldots（命令 `skd`）：声明式管理本机所有 agent 工具的 skills。Go 单文件，跨平台，库进 Git。

## 定位

现有工具（vercel `npx skills`、xingkongliang/skills-manager、SkillVault）都是"装文件到目录"或 GUI 优先、状态藏在 App 里。skilldots 的差异：

- **清单是唯一真相**：`skills.toml` 写明订阅了什么、启用到哪些 agent；`skd apply` 让电脑和清单一致
- **库自包含、进 Git**：第三方 skill 拷进库，上游改动能 diff、能回滚；换电脑 `git clone` + `skd apply`
- **仓库（pack）是管理单位，skill 是选择单位**：一个仓库 38 个 skill 不会平铺成 38 条，更新一次 fetch 覆盖全部
- **自写、私有、第三方、App 自带同等对待**，一个画面看全
- 纯 CLI，所有命令 `--json`，agent 可直接驱动；MCP、桌面端后续基于同一核心

## 三类对象

| | 说明 |
|---|---|
| Source 来源 | `github:owner/repo`、任意 git URL、`app:/绝对路径`（随软件发布，只链接不拷贝）、`skills/<名>`（自写，公开）、`private/<名>`（自写，本机独立 git 仓库，不推送） |
| Target 目标 | agent 工具的 skill 目录。内置注册表（claude `~/.claude/skills`、codex `~/.codex/skills`、gemini、cursor、opencode、agents `~/.agents/skills` …），随版本更新，`skills.toml` 可自定义 |
| State 状态 | `skills.lock`：每个 pack 的 commit、每个 skill 目录的内容哈希、实际链接/拷贝到了哪些 target |

## 库目录

```
<库>/
├── skills.toml        清单（人和 agent 都可改）
├── skills.lock        锁（工具写）
├── skills/<名>/       自写，随库公开
├── private/<名>/      自写，本机；独立 git 仓库，被外层 .gitignore 忽略
└── vendor/<owner>/<repo>/<分类>/<名>/   第三方，工具拉取，不手改；每个 repo 附 LICENSE
```

缓存：`~/.cache/skilldots/repos/<owner>__<repo>`，浅克隆，一个仓库一份，只做加速，可随时删。

## skills.toml

```toml
[targets]
default = ["agents", "claude"]        # 默认启用到哪些 agent

[targets.custom.myagent]              # 自定义 target
path = "~/.myagent/skills"

[[pack]]
repo    = "mattpocock/skills"
alias   = "matt"
use     = ["tdd", "grill-me", "code-review"]   # 或 "*" 配 skip = ["in-progress/*"]
targets = ["agents"]                            # 覆盖默认

[pack.skill.code-review]
as      = "matt-code-review"          # 重名时的链接名
targets = ["claude"]                  # 单个 skill 再覆盖

[[app]]
name = "surge"
path = "/Applications/Surge.app/Contents/Resources/Skills/surge"
targets = ["claude", "agents"]

[[local]]                             # skills/ 与 private/ 下的目录默认自动纳入，这里只写例外
name = "idea-config-maintainer"
targets = ["agents"]
```

targets 继承：全局 default → pack → skill。`targets = []` 表示保留在库里但不启用到任何 agent。

## 命令

| 命令 | 作用 |
|---|---|
| `skd init [--from 路径]` | 建库（默认 `~/.skilldots`），git init，生成清单；`--from` 接管已有仓库 |
| `skd add <来源> [--skill a,b \| --all \| --category x]` | 克隆到缓存，扫描 SKILL.md，交互选择（按分类折叠），拷入 vendor，写清单与锁，链接 |
| `skd list [pack] [--all] [--flat] [--agent x]` | 顶层按 pack 折叠一行一仓库；展开、平铺、按 agent 视角 |
| `skd apply [--dry-run]` | 使电脑与清单一致：建链接、删多余链接、拷贝模式下按哈希识别自己放的文件 |
| `skd enable <skill> [--agent a,b]` / `skd disable <skill> [--agent a,b]` | 改 targets 并 apply；不带 `--agent` 用默认集合 |
| `skd remove <skill\|pack>` | 从所有 agent 目录移除链接/拷贝，**库里保留**，清单里 targets 置空 |
| `skd purge <skill\|pack>` | 从清单、锁、vendor 彻底清除，并移除所有 agent 目录里的链接/拷贝；先列后删，`--yes` 跳过确认 |
| `skd update [pack] [--yes] [--commit]` | 按 pack fetch；报告：选中的改动（可看 diff）、未变、上游新增（提示可加）、上游删除（保留本地并标记）。`--commit` 自动提交推送 |
| `skd outdated` | 只查不改 |
| `skd new <名> [--private]` | 在 skills/ 或 private/ 生成模板 |
| `skd scan` | 扫描本机所有已知 agent 目录，列出每个 skill：来源、链接/拷贝、是否受管、重复、断链 |
| `skd adopt` | 把 scan 到的未受管 skill 收编：能识别来源的（npx `.skill-lock.json`、git remote、App 路径）自动填，否则询问 |
| `skd doctor` | 体检：Windows 符号链接权限、target 不存在、锁与清单不一致、断链 |

所有命令 `--json`；破坏性命令先列清单再确认。

## 关键规则

- **skill 根目录识别**：扫所有 `SKILL.md`，优先读仓库元数据（vercel `skills.sh.json`、`.claude-plugin`），否则取 SKILL.md 父目录为 skill 目录；分类 = skill 目录相对于公共根的路径
- **skill 名**：`SKILL.md` frontmatter 的 `name`，缺失则目录名
- **重名**：先到者占原名，后到者自动 `<alias>-<name>` 并写入 `as`，用户可改；apply 时再次校验整库无冲突
- **链接 vs 拷贝**：默认软链接；Windows 无权限或 target 要求拷贝时降级，锁里记录哈希，`apply`/`remove` 只动哈希匹配的文件，用户改过的文件不覆盖并警告
- **更新安全**：锁定 commit，`update` 永远显式；上游删除不自动删本地
- **许可证**：vendor 时拷 LICENSE；找不到（根目录、skill 目录、frontmatter `license:`）的仓库提示，自写来源不检查
- **private/**：独立 git 仓库、无远端；工具只链接、不提交、不清理

## 里程碑

1. 核心模型 + init / add / list / apply / enable / disable / remove / purge / update，用本机现状验证
2. scan / adopt / doctor；agent 注册表补全（本机 60 多个目录为第一份数据）
3. Homebrew / scoop 分发，VPS 定时更新
4. MCP server（同一核心）；桌面端 Wails（可选）

## 技术

Go 1.27；cobra；go-toml v2；纯 `git` 命令行调用（不用 go-git，减小体积，行为与用户一致）；测试用临时目录伪造 agent 目录与上游仓库。
