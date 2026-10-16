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
| State 状态 | `skills.lock`：每个 pack 的 commit、每个 skill 目录的内容哈希、上次 `update` 时上游的 skill 名单（`upstream`；首次 `add` 订阅时即记入，旧锁没有名单的 pack 在下一次 `add` 时补上）、实际链接/拷贝到了哪些 target |

## 库目录

```
<库>/
├── skills.toml        清单（人和 agent 都可改）
├── skills.lock        锁（工具写）
├── skills/<名>/       自写，随库公开
├── private/<名>/      自写，本机；独立 git 仓库，被外层 .gitignore 忽略
├── .trash/            被挪开的目录（gitignore），`skd trash` 查看与恢复
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
alias   = "matt"                                 # 不含 / \ :，不能等于已订阅的 repo
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
| `skd init [--from 路径]` | 建库（默认 `~/.skilldots`），库根和 private/ 各自 git init（即使位于更大的仓库内），生成清单；`--from` 接管已有仓库 |
| `skd add <来源> [--skill a,b \| --all \| --category x]` | 克隆到缓存，扫描 SKILL.md，交互选择（按分类折叠），拷入 vendor，写清单与锁，链接。`--skill`/`--category` 给的名字和分类先逐个校验，不存在就报错、什么都不改；`--all` 把 pack 设为 `use = ["*"]`（已有 skip 保留），同时给的 `--skill`/`--category` 只做这项校验。对已是 `"*"` 的 pack，`--skill x` 把 skip 里直接写 `x` 或 `分类/x` 的项移出；x 被通配（如 `eng/*`）排除时不拆通配，报错退出 1，提示手改 skills.toml（删掉或换成具体名字）后 `skd update <repo>` |
| `skd list [pack] [--all] [--flat] [--agent x]` | 顶层按 pack 折叠一行一仓库；展开、平铺、按 agent 视角 |
| `skd apply [--dry-run]` | 使电脑与清单一致：建链接、删多余链接、拷贝模式下按哈希识别自己放的文件 |
| `skd enable <skill> [--agent a,b]` / `skd disable <skill> [--agent a,b]` | 改 targets 并 apply；不带 `--agent` 用默认集合 |
| `skd remove <skill\|pack>` | 从所有 agent 目录移除链接/拷贝，**库里保留**，清单里 targets 置空 |
| `skd purge <skill\|pack>` | 从清单、锁、vendor 彻底清除，并移除所有 agent 目录里的链接/拷贝；先列后删，`--yes` 跳过确认 |
| `skd update [pack] [--yes] [--commit]` | 按 pack fetch；报告：选中的改动（可看 diff）、未变、上游新增（提示可加）、上游删除（保留本地并标记）。`--commit` 自动提交推送。“上游新增”= 未选中、且不在锁的 `upstream` 名单（上次 `update` 记入，从未 update 的则是 `add` 订阅时）里的 skill（旧锁没有名单时以更新前的缓存克隆为准，两者都没有则不报）；`--json` 的 `packs[].UpstreamNew` 同此含义，仪表盘 `u` 用同一份计算 |
| `skd outdated [pack]` | 只查不改：逐个 pack fetch 到缓存克隆（vendor/、skills.lock、skills.toml 不动），对比缓存 HEAD 与锁里的 commit，精确到 skill：changed（已选且已 vendor、上游内容变了）、moved（内容没变、只换了分类目录，update 只会改名，不算更新、不计入 ↑ 和 `U`，人类输出单独一行 `moved x (path only)`，仪表盘右栏标 moved）、new（不在锁的 `upstream` 名单、也没 vendor；锁里没有名单时不报，避免把缓存当基线）、gone（已 vendor、名单里有、现在上游没了）、pending（已选未 vendor，update 会拷入）。结果连同时间写入用户配置目录（与 config.toml 同目录）的 `outdated.json`，按库根目录分开记；失败的 pack 单列且保留上次记录，退出码非 0，其余照常。`skd list` 概览、问答菜单只读这份记录（不联网）：`↑ N updates available · checked 2h ago`，N 为有更新的 pack 数，超过 24 小时标 stale；锁里 commit 已变的 pack 视为未知不计。`update` 成功后把对应 pack 记为最新 |
| `skd new <名> [--private]` | 在 skills/ 或 private/ 生成模板并链接到默认 agent；名字按 skill 名规则校验（见关键规则），不合规或目录已存在则报错 |
| `skd scan` | 扫描本机所有已知 agent 目录，列出每个 skill：来源、链接/拷贝、是否受管、重复、断链 |
| `skd adopt` | 把 scan 到的未受管 skill 收编：能识别来源的（npx `.skill-lock.json`、git remote、App 路径）自动填，否则询问。提案按名字排序；断链提案在执行时重新检查，路径已变成文件、目录或有效链接时不动并告警 |
| `skd doctor` | 体检：Windows 符号链接权限、target 不存在、锁与清单不一致、断链 |

所有命令 `--json`；破坏性命令先列清单再确认。`--theme light|dark|auto`（默认 auto，也可用环境变量 `SKILLDOTS_THEME`）选配色，`NO_COLOR` 关闭颜色。

## 仪表盘

`skd` 不带参数、终端不窄于 90 列时打开全屏仪表盘（`--simple`、窄终端、`TERM=dumb` 走问答菜单）。人用仪表盘，agent 用命令；仪表盘必须覆盖命令能做的所有事，结果与对应命令一致。

| 键 | 作用 | 对应命令 |
|---|---|---|
| `space` | 启用/停用；上游未装的直接安装 | `enable` / `disable` / `add --skill` |
| `a` | 加仓库：勾选 skill 或整个分类（`*` 跟随全部），再选 alias、目标 agent，均可留空 | `add` |
| `u` / `U` | 更新当前包 / 所有检查出有更新的包（检查未成功前为全部） | `update` |
| `c` | 重新检查上游。仪表盘启动后在后台静默跑一次（不算“操作在跑”，不拒绝任何键；同一仓库的缓存克隆在进程内加锁，与 update/add 串行）：顶栏 `↑ N packs have updates`，左栏 pack 带 `↑`，右栏 `↑ changed` / `new upstream` / `gone upstream` / `moved upstream (path only)`；失败时顶栏 `check failed`，原因在状态行 | `outdated` |
| `e` | 选目标 agent | `enable/disable --agent` |
| `f` | fork，可改名，选 skills/ 或 private/ | `fork [--as] [--private]` |
| `d` | 删 vendor skill（进 trash）/ 取消链接自己的 / purge 包 | `remove` / `disable` / `purge` |
| `n` | 新建 skill，建好即链接并选中 | `new` |
| `S` | 按 agent、状态列出扫描结果，勾选后 adopt | `scan` / `adopt` |
| `t` | 回收站：恢复；清空需两次确认，第二次只认 `D` | `trash restore` / `trash empty` |
| `s` | 备份 | `sync` |
| `A` / `r` / `/` / `?` / `q` | 应用 / 重载 / 过滤 / 全部键位 / 退出 | `apply` |

有操作在跑时，会启动新操作的键一律拒绝；错误显示在状态行。

**共享流程**：所有写操作在 `internal/app`（targets 编辑与继承、取消选择、add、fork、remove/purge、enable/disable、new、adopt、update、apply、trash、sync 的 git 操作）。`internal/cli` 和 `internal/tui` 只负责收集输入和展示结果，`internal/app` 的测试用同一夹具分别走两条路径并比对清单、锁、文件和链接。

**配色**：两套调色板同一组色相（紫色强调、青色、绿/黄/红状态色）。文字分 Text、Muted、Faint 三级，依次变淡，Muted 和 Faint 在终端底色和仪表盘各层底色上都不低于 4.5:1（`TestPaletteContrast` 检查）；选中行上 Faint 按 Muted 画。边框、分隔线和各层底色的色调用单独的 Line 色，不承载文字。`auto` 时向终端发 OSC 11 加 DA1（所有终端都回 DA1，所以不支持 OSC 11 的终端也立即结束），300ms 无回应按深色；`--json`、`NO_COLOR`、`TERM=dumb`、stdout 非终端、后台进程组时不询问。询问期间终端处于 cbreak（关回显、行缓冲、ICRNL、IXON、IEXTEN，按键与仪表盘 raw 模式下读到的一致，Enter 仍是 CR；保留 ISIG：ctrl+c、ctrl+\ 照常中断，退出前恢复终端模式）；切换后若已有待读输入（用户预先敲的键，含未回车的半行）就不询问、按深色，键留给后续读取者；等待期间读到的非回复字节由仪表盘在启动时按序重放（`termbg.Unread`）；DA1 之后的字节都算按键，紧跟其后被截断的转义序列（ESC、`ESC [`）再等至多 100ms 补全。问答菜单和普通命令拿不到这些字节：不回应的终端上，在这 300ms 内敲的键对它们会丢失。Bubble Tea 的包 init 会让 termenv 询问背景并最多等 5 秒，所以 `internal/termbg` 的 init 抢先设定背景值（Go 按导入路径顺序初始化，`github.com/Maybe404/…` 排在 `github.com/charmbracelet/…` 之前）。

## 关键规则

- **skill 根目录识别**：扫所有 `SKILL.md`，取其父目录为 skill 目录；跳过测试目录和隐藏目录（`.agents` 除外），但仓库里别处没有 skill 时再扫隐藏目录（只在 `.claude/skills` 等目录发布的仓库）。同名多份：内容相同的折叠为 AltDirs，内容不同的记为 Variants，取 `skills/`、`.agents/skills/` 优先的那份，`skd show` 列出
- **分类**：skill 父路径去掉结构段（`skills`、`skill`、`plugins`、`plugin`、隐藏目录）后剩下的部分，只取决于该 skill 自己的位置；不做公共前缀剔除，也不剔除"等于名字的末段"，上游新增目录不会改变已有 skill 的分类。锁里的分类变化时 vendor 目录改名，不进 trash
- **skill 名**：`SKILL.md` frontmatter 的 `name`，缺失则目录名；必须匹配 `[A-Za-z0-9_-][A-Za-z0-9._-]*`、不含 `..`、≤128 字符，否则跳过并告警（名字会拼进 vendor 和 target 路径，上游不可信）。`as`、`[[app]].name`、`[[local]].name` 同规则；显式写空的 `name:` 也拒绝。同一仓库内只差大小写的名字、或分类会把它嵌进另一个 skill 目录的，拒绝其一并告警；符号链接形式的 LICENSE 不拷贝
- **重名**：先到者占原名；`add`/`update` 首次 vendor 的 skill 若名字已被占用，自动 `<前缀>-<name>` 并写入 `as`（前缀 = alias，否则仓库路径最后一段）。名字比较不区分大小写；同名但 target 不相交的条目可以共存。target 相交仍冲突时冲突双方都跳过、已有链接不动、告警列出两个目录，其余条目照常处理；`fork` 把 frontmatter name 改成 fork 的名字
- **单条目问题不拖垮整库**：未知 target 只跳过该 target 并告警，已记录在该 target 的链接不动；清单有 pack 而锁无记录时提示 `skd update`；锁里记录链接的绝对路径，target 被删后仍能清理
- **链接 vs 拷贝**：默认软链接；Windows 无权限或 target 要求拷贝时降级，锁里记录哈希。拷贝只有"原样"（哈希一致且没有哈希看不到的 `.git`、`node_modules`、符号链接）时才直接替换，否则整份移入 trash；用户改过的文件不覆盖并警告
- **库目录移动**：链接指向"另一个根 + 锁里记录的相对路径"时识别为旧位置的链接，`apply` 直接重建
- **不删除**：除 `skd trash empty` 外不永久删除用户数据。trash 用 rename，仅跨设备（EXDEV）时回退为完整拷贝（含 `.git`、符号链接、权限）后再删源，其他错误原样返回
- **`--json`**：stdout 只有一份 JSON，进度和告警走 stderr；失败时 stdout 为空、退出码非 0（`update` 例外：输出 `{packs, failed, apply, committed}` 后以非 0 退出；`outdated` 同理输出 `{checked, packs, failed, updates}`，`packs[]` 为 `{repo, commit, latest, changed, moved, new, gone, pending, checked}`）
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
