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
├── skills.history.jsonl  更新记录（工具追加，一行一条，随库提交推送）
├── skills/<名>/       自写，随库公开
├── private/<名>/      自写，本机；独立 git 仓库，被外层 .gitignore 忽略
├── .trash/            被挪开的目录（gitignore），`skd trash` 查看与恢复
└── vendor/<owner>/<repo>/<分类>/<名>/   第三方，工具拉取，不手改；每个 repo 附 LICENSE
```

缓存：`~/.cache/skilldots/repos/<owner>__<repo>`，浅克隆，一个仓库一份，只做加速，可随时删。每份克隆旁有 `<owner>__<repo>.lock`：fetch、扫描、拷出都在这把锁里（进程内互斥加跨进程 flock，Windows 用 LockFileEx；等锁可取消），两个 skd 同时 fetch 同一仓库时排队而不是撞 git 的 `shallow.lock`；git 因克隆 `.git/` 下的锁文件失败时（从 git 报错里引号中的路径解析出是哪个，如 `index.lock`、`shallow.lock`、`refs/heads/main.lock`，中英文输出路径原样；不在本克隆 `.git/` 下的路径不算），只有确实拿到 flock 才处理，且只处理解析出的那几个：锁文件不到 30 秒的视为有别的 git（如手动运行的）正在用，等它消失；超过 30 秒的视为被杀的 git 留下的陈旧锁，删掉；然后重试，下一次撞上别的锁文件就再按同样规则处理，最多重试 5 次。flock 拿不到（锁文件建不出、文件系统不支持）或失败原因与锁文件无关时一律不碰。

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

**自定义 target 先由人批准（首次信任）**：`skills.toml` 可能来自别人的库（`skd clone`），自定义 target 的路径由对方决定，而 skd 会往 target 里建链接或拷贝、把回收站条目恢复到那里、`--force` 时把挡路的东西挪进回收站。所以自定义 target 只在两个条件都满足时生效，否则在 apply、enable、add、update、restore、clone、`skd targets`、仪表盘里都当作不存在（`Registry` 不收录，`Get`/`All`/`Present` 看不到，记在 `Withheld` 里）：

1. **路径允许**（`targets.CheckPath`，批准了也必须满足）：`~` 展开、解析已存在部分的符号链接后，必须是绝对路径、严格位于 `$HOME` 之下（不是 `$HOME` 本身）；不能是、不能位于、也不能包含下列敏感位置（相对 `$HOME`）：`.ssh`、`.gnupg`、`.aws`、`.azure`、`.kube`、`.docker`、`.password-store`、`.config/gcloud`、`.config/gh`、`.config/git`、`.config/autostart`、`.config/systemd`、`.config/environment.d`、`.local/bin`、`bin`、`Library/LaunchAgents`、`Library/Keychains`、`AppData/Roaming/Microsoft/Windows/Start Menu/Programs/Startup`，以及 skd 自己的设置目录（config.toml 与批准记录所在处）；不能包含某个内置 target 的目录（如 `~/.claude`，那里有 agent 自己的设置；等于内置 target 目录可以）。“包含”也挡：target 是 `~/Library` 时，名为 `LaunchAgents` 的 skill 会落到 `~/Library/LaunchAgents`。比较时逐级按同一文件（`os.SameFile`）判断，拒绝时还忽略大小写（macOS、Windows 默认不区分大小写，`~/.SSH` 就是 `~/.ssh`）。自定义名与内置名相同一律拒绝：内置 target 照常生效，自定义的那条不用，每次读库都告警（以前同名的自定义 target 会覆盖内置的，只用默认 target 的库也被重定向）。
2. **人批准过**：批准记录是 `(名字, 展开并解析链接后的真实路径)`，存在库外、与 config.toml 同目录的 `trusted-targets.toml`（macOS `~/Library/Application Support/skilldots/`，Linux `~/.config/skilldots/`），不进库、不随 clone 来；格式为若干 `[[target]]`，各有 `name`、`path`（真实路径）、`approved`（UTC 时间）。名字或真实路径任一变了（库里改了 `path`，或路径上的符号链接改指别处）都要重新批准。批准只能由人在交互终端完成：`skd targets trust <名>` 先显示 skills.toml 里的写法、真实目录与模式，确认后才记录（写入前再算一次真实路径，与显示的不同就不记）；stdin 或 stdout 不是终端、`--yes`、`--json` 时一律拒绝，并告诉 agent 请用户在终端运行这条命令。仪表盘 `T` 列出所有 target，`a` 批准选中的自定义 target（同样先显示再确认）。`skd targets untrust <名>` 撤销（不需要终端，撤销只会让 skd 少写），已建的链接留在原处，直到把该 target 从清单删掉再 apply。批准记录文件读不了时所有自定义 target 都不生效，也不能再批准，直到修好或删掉它。

`skd clone` 在 apply 之前列出库里的每个自定义 target 和不用它的原因：能批准的，交互终端下逐个显示并询问（默认否），非交互（含 `--yes`、`--json`）时跳过并说明如何批准；路径不允许或与内置同名的只说明原因。之后每个读库的命令都在 stderr 告警每个不生效的自定义 target（原因、受影响的 skill），apply 的计划里该 target 的条目显示为跳过（“custom target … is not used”），锁里已记录在它下面的链接不动。已知局限：能写用户配置目录、或能伪造终端（如 `script`）的程序仍可绕过；这一层挡的是“clone 一个库、无人值守地执行命令”，不是本机上的恶意程序。

## 命令

| 命令 | 作用 |
|---|---|
| `skd init [--from 路径]` | 建库（默认 `~/.skilldots`），库根和 private/ 各自 git init（即使位于更大的仓库内），生成清单；`--from` 接管已有仓库 |
| `skd add <来源> [--skill a,b \| --all \| --category x]` | 来源除 owner/repo、GitHub URL、git URL（`@ref` 锁定）外，还接受：整行粘贴的安装命令（`npx`/`bunx`/`pnpx`/`pnpm dlx`/`yarn dlx`/`npm exec`/`bun x` + `skills add`（或 `a`/`install`/`i`）或 `add-skill`，以及 `skd add`；`-s`/`--skill`/`--skills` 可重复、可逗号分隔、一个 flag 后可跟多个名字，`-a`/`--agent` 的值被跳过，其他 flag 忽略；遇到 `&&` `||` `;` `|` 截断；只取第一行，去掉行首提示符）、skills.sh 页面（`skills.sh/<owner>/<repo>[/<skill>]`）、GitHub `/tree/<ref>/<路径>`（skill 目录或包含多个 skill 的目录）与 `/blob/<ref>/…`（`SKILL.md` 或 skill 内任意文件）。文本只按 shell 规则分词，不展开、不执行。命令行上的多个参数只在第一个是安装命令词（npx、bunx、skd 等，即未加引号粘贴的命令）时拼成一行，否则只接受一个来源，多余参数在 fetch 之前报错（不留缓存和锁文件）。以 `-` 开头的来源或 `@ref` 一律拒绝（git 会把它当选项），`pack.Fetch` 的 clone/fetch 以及 `skd clone` 也在 URL 和 ref 前加 `--`，手改 skills.toml 写进去的也不会被当成选项。解析出仓库后照常 fetch，链接/参数指定的 skill 在选择界面预选（名字按 frontmatter 名或目录名匹配；路径按仓库内相对目录匹配，ref 含 `/` 时从路径前部逐段丢弃再试），匹配不上的告警。**链接里的 ref 不锁定**：那只是浏览位置，pack 跟随默认分支，与 `owner/repo` 一致，要锁定用 `@ref`。无终端（或 `--json`/`--yes`）且未给 `--skill`/`--category`/`--all` 时，预选的 skill 就是选择；指定的名字都不存在时报错。交互终端不带参数时提示输入来源。之后：克隆到缓存，扫描 SKILL.md，交互选择（按分类折叠），拷入 vendor，写清单与锁，链接。`--skill`/`--category` 给的名字和分类先逐个校验，不存在就报错、什么都不改；`--all` 把 pack 设为 `use = ["*"]`（已有 skip 保留），同时给的 `--skill`/`--category` 只做这项校验。对已是 `"*"` 的 pack，`--skill x` 把 skip 里直接写 `x` 或 `分类/x` 的项移出；x 被通配（如 `eng/*`）排除时不拆通配，报错退出 1，提示手改 skills.toml（删掉或换成具体名字）后 `skd update <repo>` |
| `skd list [pack] [--all] [--flat] [--agent x]` | 顶层按 pack 折叠一行一仓库；展开、平铺、按 agent 视角 |
| `skd apply [--dry-run]` | 使电脑与清单一致：建链接、删多余链接、拷贝模式下按哈希识别自己放的文件 |
| `skd enable <skill> [--agent a,b]` / `skd disable <skill> [--agent a,b]` | 改 targets 并 apply；不带 `--agent` 用默认集合 |
| `skd remove <skill\|pack>` | 从所有 agent 目录移除链接/拷贝，**库里保留**，清单里 targets 置空 |
| `skd purge <skill\|pack>` | 从清单、锁、vendor 彻底清除，并移除所有 agent 目录里的链接/拷贝；先列后删，`--yes` 跳过确认 |
| `skd update [pack] [--yes] [--commit]` | 交互终端且无 `--yes`/`--json` 时先确认：对给定的 pack（不给为全部）跑一次与 `outdated` 相同的检查，逐包列出 skill 级变化（changed / 将加入 / 上游新增 / 上游删除 / moved），默认全选，可逐个取消，回车只更新勾选的包，ctrl+c 取消；未勾选的包 vendor、锁、链接都不动。不给 pack 时只列 `HasUpdates` 的包（只有 moved 或只有新 commit 的不列），点名的 pack 只要 commit 变了或有 moved 也列。确认后 `update` 重新 fetch，确认与执行之间上游再变的以执行时为准。点名的 pack 上游没有任何变化时，先说明再问“仍然更新？”（与仪表盘 `u` 对等：重新 vendor 缺失的 skill 并补链接）；不点名时提示 `skd apply` 补链接、`skd update <pack>` 强制重来。非交互、`--yes`、`--json` 行为不变（agent 路径）；“交互”要求 stdin 和 stdout 都是终端（所有命令统一），`skd update | cat` 这类 stdout 被管道接走的调用按非交互处理，不会弹出看不见的确认而挂住。之后：按 pack fetch；报告：选中的改动（可看 diff）、未变、上游新增（提示可加）、上游删除（保留本地并标记）。`--commit` 自动提交推送。“上游新增”= 未选中、且不在锁的 `upstream` 名单（上次 `update` 记入，从未 update 的则是 `add` 订阅时）里的 skill（旧锁没有名单时以更新前的缓存克隆为准，两者都没有则不报）；`--json` 的 `packs[].UpstreamNew` 同此含义，仪表盘 `u` 用同一份计算 |
| `skd history [pack] [--json] [-n N]` | 更新记录：每次 update 成功后，对 commit 变了或 skill 有变化的每个 pack 追加一条到库根的 `skills.history.jsonl`（不在 gitignore 里，`--commit`/`sync` 一并提交推送，换机器 clone 后可见）：时间（UTC）、pack、旧→新 commit、changed / added / moved / gone / dropped / new（上游新增未选）、上游在这些 skill 目录下的提交标题（`git log --no-merges 旧..新 -- 目录`，gone 的按 `**/<名>/**`，最多 20 条，超过标 `moreCommits`）。缓存克隆是浅克隆：旧 commit 不是新 commit 的祖先时先 `fetch --deepen=200`，仍不可达（上游改写了历史）标 `commitsUnknown`。上游提交标题、pack 名和 skill 名在写入时去掉控制字符（ESC、BEL、C1、改变文字方向的 bidi 控制符；制表和换行变空格）并截断（标题与名字各 200 字符），读取时再清洗一遍（旧版本写的、git 合并进来的行），所以 CLI、`--json`、仪表盘都不会把上游的控制序列写到终端。追加前若文件末尾没有换行先补一个，避免和手工编辑留下的最后一行粘连。读记录只读库内文件，无网可用；写入的每行不超过 256 KiB（读端上限的四分之一）：超长时反复把最长的 skill 名单砍掉后一半，砍掉的个数记在 `omitted`（CLI 与仪表盘显示为 “not recorded N more skill names”），名单都空了再砍提交标题（标 `moreCommits`），所以写下的每一行都能读回。读不懂的行（合并冲突残留）和超过 1 MiB 的行跳过并计入 `skipped` 告警，不会让整个 history 失败；按时间倒序（两台机器的记录合并后可能乱序），同一秒内的按文件中后写的在前。update 保存锁文件之后 apply（链接）失败时仍然记录，并在 `applyError` 里写明（vendor 与锁已经到新 commit，更新已经发生，`skd apply` 补完链接；不记的话这次变化就从历史里消失了）；之后一次 apply 成功（含无事可做、以及 enable/disable 等内部调用的 apply）且某个 pack 的 skill 全部链上（计划里没有该 pack 的 skip：挡路、被改动、重名、源目录缺失、未知 target），就把该 pack 记录里的 `applyError` 去掉；还有 skill 被跳过的 pack 保留（只改这些行，行内其他字段、包括不认识的字段保留，其余行逐字节不变；原子替换。从读文件到替换全程持有 history 锁：缓存目录里按库的真实路径命名的文件锁（flock / LockFileEx）加进程内锁，追加记录同样持锁，所以另一个 skd 进程或协程同时追加的记录会等替换完成再写入新文件，不会随旧文件丢失；选择加锁而不是追加 settled 记录由读端折叠，是因为旧版本 skd 会把不认识的 settled 行当坏行告警），历史不再提示“link failed”；锁文件保存失败时不记录（锁仍是旧 commit，下次 update 从旧 commit 重新比较并记录）。默认显示 20 条，`-n 0` 全部。仪表盘 `h` |
| `skd edit <skill> [--fork [--private] [--as 名] \| --force]` | 用编辑器打开 skill 目录。编辑器：配置项 `editor` → `$VISUAL` → `$EDITOR` → 自动探测 VS Code、Cursor、Zed、Sublime（PATH 里的 `code`/`cursor`/`zed`/`subl`，再看 macOS 的 `/Applications`、`~/Applications` 下的 .app，用 `open -a`）→ 系统默认（macOS `open`、Linux `xdg-open`、Windows `explorer`）。命令按 shell 规则分词但不展开、不经 shell：双引号内反斜杠只转义 `$`、反引号、`"`、`\` 和换行（同 POSIX），其余原样保留；Windows 上反斜杠不作转义（它是路径分隔符，`"C:\Program Files\…\code.cmd"` 原样可用）。已知窗口型编辑器（code、cursor、zed、subl、open 等）后台启动不等待；其余视为终端编辑器，除非设置 `editor-kind`：`window` 把配置的命令（设置、`$VISUAL`、`$EDITOR`）当窗口型，`terminal` 当终端型，`auto`/不设按名单判断；无终端时的报错给出 `skd config set editor-kind window`，CLI 前台运行，仪表盘交出终端（`tea.ExecProcess`），无终端时报错。编辑器命令找不到、或终端型编辑器遇上无终端（含 `--json`）时，在 fork 之前就报错，库不变（`EditOptions.NoTerminal`）。vendor/app 来源的 skill 更新时会被覆盖：交互时提示并给出“fork 到 skills/ 再打开”“fork 到 private/ 再打开”“仍然打开”；非交互（无终端、`--json`、`--yes`）时拒绝，必须显式 `--fork`（走现有 fork 流程后打开副本）或 `--force`（打开 vendor 里的文件）。仪表盘 `o` |
| `skd config list \| get <键> \| set <键> [值]` | 用户设置，存在现有用户配置文件 config.toml（与 `library` 同一文件，写入时保留其他键和表，包括不认识的：整份文件一次序列化，顶层键始终在所有 `[表]` 之前，键的归属不变，写前先确认能读回；文件损坏时拒绝写入。只改该键所在的那一行（新键插在顶层最后一个非空行之后、表头前的注释块之前），用户的注释、空行、表原样保留，该行行尾的注释也保留（从第一个使该行前半段恰好解析为这个键的 `#` 起算，值里的 `#` 不算；删除键时整行连同注释删去）；换行风格跟随文件（首行是 CRLF 则改动行、新行和整份重写都用 CRLF）；改完必须解析回来恰好等于预期的设置，否则（如多行字符串值）退回整份重写，此时注释丢失。文件不是合法 TOML 时（`config.InvalidError`，带路径和解析错误）每种情形只说一次：需要它的命令把它作为错误报出——找库走到“记住的库”这一步（提示用 `--library` 或 `$SKILLDOTS_LIBRARY`，不再退回默认目录或误报“没有库”）、`config list/get/set`、`use`，以及 `init`/`clone` 在建库或克隆**之前**就失败（它们最后要把库记进这个文件，先检查，免得留下半成品）；命令没用到它而照常完成（比如 `--library` 指定了库）时，结束后在 stderr 告警一行“它的设置（editor、theme…）都没有用上”）：`editor`、`editor-kind`（`auto`/`window`/`terminal`，见 `skd edit`）、`theme`（`auto`/`light`/`dark`，优先级 `--theme` > `$SKILLDOTS_THEME` > 设置 > auto；设置里的值非法时按 auto 处理并在 stderr 告警一次，不让任何命令失败，`skd config set theme` 可以修复）、`lang`（预留，`auto`/`en`/`zh`，暂无翻译）；`library` 只读显示，用 `skd use` 改。不给值即删除该键。仪表盘 `,` |
| `skd outdated [pack]` | 只查不改：逐个 pack fetch 到缓存克隆（vendor/、skills.lock、skills.toml 不动），对比缓存 HEAD 与锁里的 commit，精确到 skill：changed（已选且已 vendor、上游内容变了）、moved（内容没变、只换了分类目录，update 只会改名，不算更新、不计入 ↑ 和 `U`，人类输出单独一行 `moved x (path only)`，仪表盘右栏标 moved）、new（不在锁的 `upstream` 名单、也没 vendor；锁里没有名单时不报，避免把缓存当基线）、gone（已 vendor、名单里有、现在上游没了）、pending（已选未 vendor，update 会拷入）。结果连同时间写入用户配置目录（与 config.toml 同目录）的 `outdated.json`，按库根目录分开记；失败的 pack 单列且保留上次记录，退出码非 0，其余照常；文件损坏时告警一次并重写。`skd list` 概览、问答菜单只读这份记录（不联网）：`↑ N updates available · checked 2h ago`，N 为有更新的 pack 数，超过 24 小时标 stale；锁里 commit 已变的 pack 视为未知不计。`update` 成功后把对应 pack 记为最新 |
| `skd new <名> [--private]` | 在 skills/ 或 private/ 生成模板并链接到默认 agent；名字按 skill 名规则校验（见关键规则），不合规或目录已存在则报错 |
| `skd scan` | 扫描本机所有已知 agent 目录，列出每个 skill：来源、链接/拷贝、是否受管、重复、断链 |
| `skd adopt` | 把 scan 到的未受管 skill 收编：能识别来源的（npx `.skill-lock.json`、git remote、App 路径）自动填，否则询问。提案按名字排序；断链提案在执行时重新检查，路径已变成文件、目录或有效链接时不动并告警 |
| `skd targets [trust <名> \| untrust <名>]` | 列出内置与自定义 target、是否存在、是否生效（不生效的自定义 target 带原因；`--json` 的 `Active`、`Approvable`、`Problem`）；`trust` 由人在终端批准自定义 target，`untrust` 撤销（见上文“自定义 target 先由人批准”） |
| `skd doctor` | 体检：Windows 符号链接权限、target 不存在、锁与清单不一致、断链 |

所有命令 `--json`；破坏性命令先列清单再确认。`--theme light|dark|auto`（默认 auto，也可用环境变量 `SKILLDOTS_THEME`，再次是 `skd config set theme`）选配色，`NO_COLOR` 关闭颜色。

## 仪表盘

`skd` 不带参数、终端不窄于 90 列时打开全屏仪表盘（`--simple`、窄终端、`TERM=dumb` 走问答菜单）。人用仪表盘，agent 用命令；仪表盘必须覆盖命令能做的所有事，结果与对应命令一致。对话框在矮终端上先省掉说明，再截掉正文，按键提示行（每个键做什么）始终保留；窄到一行放不下时按键提示折成多行。

| 键 | 作用 | 对应命令 |
|---|---|---|
| `space` | 启用/停用；上游未装的直接安装 | `enable` / `disable` / `add --skill` |
| `a` | 加仓库：勾选 skill 或整个分类（`*` 跟随全部），再选 alias、目标 agent，均可留空 | `add` |
| `u` / `U` | 先检查上游并展示将变化什么：`u` 当前包，确认后更新（无变化也可确认，用于补回缺失文件）；`U` 列出有更新的包及 skill 级变化，默认全选、可取消，只更新勾选的。检查结果并入顶栏和右栏标记 | `update` |
| `h` | 更新记录列表，回车看某条的 skill 变化与上游提交标题 | `history` |
| `c` | 重新检查上游。仪表盘启动后在后台静默跑一次（不算“操作在跑”，不拒绝任何键；同一仓库的缓存克隆加锁，与 update/add 及其他 skd 进程串行；退出时取消检查：git 及其子进程收到 SIGTERM（独立 session、独立进程组，git 借此清理锁文件），1 秒后仍在的被杀，退出最多等 2 秒，不留 git 进程；独立 session 没有控制终端，要从 /dev/tty 读口令的 core.sshCommand 直接失败、显示为 check failed，而不是被挂起；终端挂断（SIGHUP，如关窗口、`tmux kill-session`）时仪表盘关闭，按退出流程取消检查并立即停掉在跑的操作，skd 以非 0 退出）：顶栏 `↑ N packs have updates`，左栏 pack 带 `↑`，右栏 `↑ changed` / `new upstream` / `gone upstream` / `moved upstream (path only)`；失败时顶栏 `check failed`，原因在状态行 | `outdated` |
| `e` | 选目标 agent | `enable/disable --agent` |
| `f` | fork，可改名，选 skills/ 或 private/ | `fork [--as] [--private]` |
| `o` | 用编辑器打开 skill；vendor/app 来源先问：fork 后打开 / 仍然打开 | `edit` |
| `d` | 删 vendor skill（进 trash）/ 取消链接自己的 / purge 包 | `remove` / `disable` / `purge` |
| `n` | 新建 skill，建好即链接并选中 | `new` |
| `S` | 按 agent、状态列出扫描结果，勾选后 adopt | `scan` / `adopt` |
| `t` | 回收站：恢复；清空需两次确认，第二次只认 `D` | `trash restore` / `trash empty` |
| `T` | target 列表，不生效的自定义 target 在前并注明原因；`a` 批准能批准的那个（先显示真实目录再确认） | `targets` / `targets trust` |
| `s` | 备份 | `sync` |
| `,` | 设置页：editor、theme（下次启动生效）、lang | `config` |
| `A` / `r` / `/` / `?` / `q` | 应用 / 重载 / 过滤 / 全部键位 / 退出 | `apply` |

`h` 原为“切到左栏”，现改为更新记录；切栏用 `←`、`tab`。

**Claude Code 里关闭的 skill**：Claude Code 把用户在它里面关掉的 skill 记在 `~/.claude/settings.json`（设置了 `$CLAUDE_CONFIG_DIR` 时为其下的 `settings.json`）的 `skillOverrides`，形如 `{"name": "off"}`（`"disabled"` 或 `false` 也算）。skd 只读、**绝不写**该文件，缺失或格式错误静默忽略；只按链接名（Claude 在目录里看到的名字）匹配，且只标装到 claude 目标（或目录就是 Claude 的 `skills/` 的自定义目标）的条目：另一个包的同名 skill 以别名链接、或者根本不链接给 Claude 的，都不标；在 `skd list`、仪表盘左右栏和详情里标 “off in Claude Code”，`list --json` 每项多 `ClaudeDisabled`。skd 照常链接；要彻底不装到 Claude，用 `e` 去掉 claude 目标（`skd disable <skill> --agent claude`）。

有操作在跑时，会启动新操作的键一律拒绝；错误显示在状态行。

**共享流程**：所有写操作在 `internal/app`（targets 编辑与继承、取消选择、add（含粘贴来源解析 `ParseAddText` 与预选 `Preselect`）、fork、remove/purge、enable/disable、new、adopt、update（含 `PlanUpdate` 与更新记录）、history、edit（编辑器选择 `FindEditor`）、Claude 关闭状态、apply、trash、sync 的 git 操作）；用户设置在更底层的 `internal/config`。`internal/cli` 和 `internal/tui` 只负责收集输入和展示结果，`internal/app` 的测试用同一夹具分别走两条路径并比对清单、锁、文件和链接。

**配色**：两套调色板同一组色相（紫色强调、青色、绿/黄/红状态色）。文字分 Text、Muted、Faint 三级，依次变淡，Muted 和 Faint 在终端底色和仪表盘各层底色上都不低于 4.5:1（`TestPaletteContrast` 检查）；选中行上 Faint 按 Muted 画。边框、分隔线和各层底色的色调用单独的 Line 色，不承载文字。`auto` 时向终端发 OSC 11 加 DA1（所有终端都回 DA1，所以不支持 OSC 11 的终端也立即结束），300ms 无回应按深色；`--json`、`NO_COLOR`、`TERM=dumb`、stdout 非终端、后台进程组时不询问。询问期间终端处于 cbreak（关回显、行缓冲、ICRNL、IXON、IEXTEN，按键与仪表盘 raw 模式下读到的一致，Enter 仍是 CR；保留 ISIG：ctrl+c、ctrl+\ 照常中断，退出前恢复终端模式）；切换后若已有待读输入（用户预先敲的键，含未回车的半行）就不询问、按深色，键留给后续读取者；等待期间读到的非回复字节由仪表盘在启动时按序重放（`termbg.Unread`）；DA1 之后的字节都算按键，紧跟其后被截断的转义序列（ESC、`ESC [`）再等至多 100ms 补全。问答菜单和普通命令拿不到这些字节：不回应的终端上，在这 300ms 内敲的键对它们会丢失。Bubble Tea 的包 init 会让 termenv 询问背景并最多等 5 秒，所以 `internal/termbg` 的 init 抢先设定背景值（Go 按导入路径顺序初始化，`github.com/Maybe404/…` 排在 `github.com/charmbracelet/…` 之前）。

## 关键规则

- **skill 根目录识别**：扫所有 `SKILL.md`，取其父目录为 skill 目录；跳过测试目录和隐藏目录（`.agents` 除外），但仓库里别处没有 skill 时再扫隐藏目录（只在 `.claude/skills` 等目录发布的仓库）。同名多份：内容相同的折叠为 AltDirs，内容不同的记为 Variants，取 `skills/`、`.agents/skills/` 优先的那份，`skd show` 列出
- **分类**：skill 父路径去掉结构段（`skills`、`skill`、`plugins`、`plugin`、隐藏目录）后剩下的部分，只取决于该 skill 自己的位置；不做公共前缀剔除，也不剔除"等于名字的末段"，上游新增目录不会改变已有 skill 的分类。锁里的分类变化时 vendor 目录改名，不进 trash
- **skill 名**：`SKILL.md` frontmatter 的 `name`，缺失则目录名；必须匹配 `[A-Za-z0-9_-][A-Za-z0-9._-]*`、不含 `..`、≤128 字符，否则跳过并告警（名字会拼进 vendor 和 target 路径，上游不可信）。`as`、`[[app]].name`、`[[local]].name` 同规则；显式写空的 `name:` 也拒绝。同一仓库内只差大小写的名字、或分类会把它嵌进另一个 skill 目录的，拒绝其一并告警；符号链接形式的 LICENSE 不拷贝
- **重名**：先到者占原名；`add`/`update` 首次 vendor 的 skill 若名字已被占用，自动 `<前缀>-<name>` 并写入 `as`（前缀 = alias，否则仓库路径最后一段）。名字比较不区分大小写；同名但 target 不相交的条目可以共存。target 相交仍冲突时冲突双方都跳过、已有链接不动、告警列出两个目录，其余条目照常处理；`fork` 把 frontmatter name 改成 fork 的名字
- **单条目问题不拖垮整库**：未知 target 只跳过该 target 并告警，已记录在该 target 的链接不动；清单有 pack 而锁无记录时提示 `skd update`；锁里记录链接的绝对路径，target 被删后仍能清理。锁由库带来、可被伪造，所以 `apply`（含 `--force`）要移走的位置从不只看锁里的 `path`：target 在用时一律是 target 目录拼上记录名（名字须通过 `CheckName`）；target 不在用（被删、未批准、路径不允许）时，`path` 必须是干净的绝对路径、末段等于记录名：父目录（解析链接后）是某个在用 target 的目录时，按那个 target 的直接子项处理；否则只删自己建的、仍指向库内（或 `[[app]]`）的链接，`--force` 不适用；其余只忘掉记录、磁盘不动。所以伪造的 `links."evil/Documents"`（path 指向 `~/Documents`）在任何情况下都挪不走 `~/Documents`
- **链接 vs 拷贝**：默认软链接；Windows 无权限或 target 要求拷贝时降级，锁里记录哈希。拷贝只有"原样"（哈希一致且没有哈希看不到的 `.git`、`node_modules`、符号链接）时才直接替换，否则整份移入 trash；用户改过的文件不覆盖并警告
- **库目录移动**：链接指向"另一个根 + 锁里记录的相对路径"时识别为旧位置的链接，`apply` 直接重建
- **不删除**：除 `skd trash empty` 外不永久删除用户数据。trash 用 rename，仅跨设备（EXDEV）时回退为完整拷贝（含 `.git`、符号链接、权限）后再删源，其他错误原样返回
- **恢复只回到 skd 移走东西的地方**：每个回收站条目是 `.trash/<批次>/<名>` 加旁边的 `<名>.origin`（一行绝对路径，移入时记录；库或 target 以相对路径给出时也先转成绝对路径）。别人 `skd clone` 来的库里 `.trash/` 可能被 force-add 进 git，`.origin` 和目录结构都由对方决定，所以 `skd trash restore <id>` 只认 skd 自己可能写出的条目：①id 恰为两段 `<批次>/<名>`，每段非空、不是 `.`/`..`、不含 `/`、`\`、NUL；`.trash`、批次目录、条目本身逐级 Lstat，必须是真目录，任何一级是符号链接都拒绝（链接可以把“回收站里的条目”指到库外任意目录）；②`.origin` 必须是普通文件且不超过 16 KiB（FIFO 或指向 `/dev/zero` 的链接会让读取卡死）；③记录的路径不含控制字符、是绝对路径、`filepath.Clean` 后不变（不含 `..`、`.`、重复或末尾分隔符）；④位置只能是 skd 移走东西的两类地方——某个已知 target（内置注册表加 `skills.toml` 的自定义 target）目录的**直接子项**（`apply --force`、`disable`/`remove` 时挡路的目录、`adopt` 挪开的原目录），或库内 `vendor/` 之下（`update` 挪开改过的 vendor skill 与上游删掉的 skill、`remove`/`purge` 移走的 skill 或整个 pack）。比较前先解析路径上已存在部分的符号链接：经由链接落到 target 目录里算 target；`vendor/` 本身是链接、或 `vendor/` 内的链接指向外面都不算 vendor。`skills/`、`private/` 不在内：skd 从不把那里的东西移进回收站，指向那里的记录不是 skd 写的。自定义 target 只算已批准、路径允许的（见“自定义 target 先由人批准”；未批准的不在注册表里，指向它的条目不可恢复），且条目名必须通过 `manifest.CheckName`（不能以点开头等）：`apply` 只会往那里写这样的名字，恢复不能比 `apply` 多出能力（以前不限制，库里定义 `path = "~"` 的 target 再配一个 `.ssh` 条目就能写出 `~/.ssh/authorized_keys`）；内置 target 不加名字限制。不合规的条目在 `skd trash`（表格后逐条说明原因）、`--json`（`restorable: false` 与 `problem`）和仪表盘回收站（“cannot be restored”，按 restore 只显示原因，不启动任务）里标出，restore 拒绝并提示手动处理；报错里的路径与 id 用 Go 转义引号输出，不带控制字符。目标已存在时仍然拒绝，从不覆盖。代价：库目录改名或换了（非符号链接的）访问路径后，旧条目的 vendor 来源对不上，需要手动移回；某个自定义 target 从清单删掉后，来自它的条目同样如此
- **锁内容不进 git 选项位**：`skills.lock` 读入时（`lock.Load`）每个 pack 的 `commit` 必须为空或 7–64 位十六进制，否则报错（带文件路径与 pack 名，提示 `git checkout -- skills.lock`）：commit 会传给 git，恶意锁写 `commit = "--output=/路径"` 就能让 `git log` 覆盖文件。传 commit 给 git 的调用另加 `--end-of-options`（仪表盘读 commit 时间的 `git log`、history 的 `merge-base` 与 `log`），这需要 git ≥ 2.24（README 写明）；更旧的 git 报不认识它时，`library.GitContext` 去掉它重试一次，前提是它之后、`--` 之前没有任何以 `-` 开头的参数（commit 已是校验过的十六进制，不会被当成选项），否则在 git 的报错后补一行“skd needs git 2.24 or later”；ref、URL、remote 一律放在 `--` 之后
- **终端输出不带外来控制字符**：上游仓库、库文件、agent 目录、回收站、设置文件里的文字写到终端前都不带控制字符（C0、DEL、C1）和 bidi 控制符，统一由 `internal/safetext` 判断与清洗（`Clean` 单行、`Lines` 保留换行、`Screen` 终端兜底、`Doc` 查整份文档），`app.CleanText` 与仪表盘的 `clean` 都是它。分三层：①**加载边界，标识类拒绝**：`manifest.Load` 与 `lock.Load` 把整份 TOML 解成通用树（`safetext.DocSchema`）：skd 读取的位置（清单的 targets、自定义 target 名与 path/mode、pack 的 repo/ref/alias/use/skip/targets、skill 覆盖的键与 as/targets、app、local；锁的 pack 名、commit、ref、upstream、skill 名、分类、哈希、链接键与各字段）里的字符串值、数组元素、表键有这类字符（含制表、换行）就整份拒绝；不认识的表和键（用户自己加的笔记、新版本的字段）只拒绝绝不会是文字的字符（ESC、BEL 等其余 C0、DEL、C1、bidi 控制符、非法 UTF-8），放过制表、换行、回车，多行字符串的备注不会让所有命令不可用（这些位置 skd 从不打印，保存时也不写回），报错逐个列出位置与值（`pack[0].use[1] = "…"`、`key pack[0].skill."…"`，均以 Go 转义引号输出，本身不含控制字符）；`Save` 前做同样检查，skd 永不写出自己会拒绝的文件；`manifest.CheckCategory` 拒绝这类分类（扫描时该 skill 被拒并告警）；`pack.ParseSource`、`--alias`（`CheckAlias`）、`config set`、`sync --remote`、`clone` 拒绝这类输入；`trash restore` 不恢复到含这类字符的来源路径；`adopt` 跳过名字不是合法 skill 名的条目（否则会建出同名目录与链接）。②**展示类清洗**：`skillmd.Read` 清洗 description/license（`--json` 里也是清洗后的）；history 写入与读取都清洗；CLI 打印外来文字（告警、错误 `Main`、进度行 `logLine`、update 报告、scan/adopt 条目与提案、apply 计划的 target/名字/原因/挪开的路径、trash 的 ID 与来源、restore 结果、config 的值、提示里的选项）走 `txt`/`errText`；仪表盘 `paint`/`fit` 清洗每段文字，输入框的初始值（如设置里的 editor）也清洗。③**打印层兜底**：CLI 给人看的输出（stdout/stderr，不含 `--json` 的 stdout）经 `screenWriter`（`safetext.Screen`：只放行换行和 SGR 颜色序列，关颜色时连 SGR 也去掉；SGR 只放行 skd 绘制会用到、不会把文字藏起来的参数：重置、粗体、暗淡、斜体、下划线（含 `4:x` 样式）、反显（输入框光标）及其关闭，前景/背景色（16 色、256 色、RGB，含冒号写法）及默认色；含隐藏（8）、闪烁（5、6）或其他参数的整条丢弃；按流记住当前前景与背景色（`Screener`，CLI 的 writer 跨多次写入保持），一条序列会让前景与背景成为同一颜色（如 `ESC[30;40m`，或分两条先设前景再设同色背景）也丢弃。已知限制：只比较完全相同的颜色，无法知道终端默认背景，所以和终端底色相近的前景色挡不住，这类外来序列靠前两层拦截。副作用：浅色主题在 256/16 色终端上，调色板降级会把部分 Muted/Faint 文字与所在底色量化成同一色（本来就看不见），这些段丢掉颜色，以终端默认色显示；跨两次写入的 UTF-8 字符先缓存，进程退出前（`Main` 返回时）把仍缓存的半个字符冲刷出去，显示为 U+FFFD，不丢末尾字节），仪表盘 `View` 同样过 `Screen`；仪表盘关闭后直接写到终端的消息（ctrl+c 时任务的结局、“waiting for/stopping <任务>”，任务标签可能带回收站条目名或 pack 名）经 `tui` 的 `termOut`/`termErr`（`cleanWriter`，同样是 `Screen`，不放行颜色），默认构建与 rawoutput 构建都有。不经过兜底的只有交给别的程序的终端：`skd edit` 前台运行的终端编辑器，skd 自己发给终端的背景色询问（OSC 11、DA1），以及 huh 问答提示（它们画光标移动，不能整体过滤；提示里的外来文字由调用方清洗，`TestHostilePromptsDrawClean` 检查）。回归闸门：`internal/hostile` 造一个处处带 ESC/BEL/C1/bidi 的库（上游 SKILL.md 各字段、目录与分类名、提交标题、history 每个字段、回收站目录名与 .origin、诱导越界恢复的回收站条目（库外目录配上 `..` 的 id、符号链接的批次与条目、来源在 `~/Library` 下、相对路径、含 `..`、库内 `skills/`；`TestHostileLibraryPrintsClean` 与仪表盘闸门逐个尝试恢复，要求全部被拒且不产生、不挪走任何目录）、agent 目录条目、设置文件），`TestHostileLibraryPrintsClean` 以不带兜底的构建（`-tags rawoutput`）依次跑每个子命令（含 dry-run、错误路径、`--json` 时的 stderr），`TestHostileCommandsCoverTheTree` 要求命令树里每个命令都在表里，`TestHostileManifestAndLockAreRefused` 逐字段验证拒绝与报错，`TestHostilePromptsDrawClean` 在伪终端里看提示，`TestHostileLibraryDrawsClean` 对仪表盘各视图的 `view`（兜底之前）做同样断言。`--json` 其余字段保持原样（数据；`encoding/json` 转义 U+0020 以下的控制字符与 U+2028/2029，skd 再把 DEL、C1 和 bidi 控制符写成 `\uXXXX`（`safetext.JSON`），解析结果不变，把 JSON 直接显示在终端上也不会被重排或驱动终端；hostile 闸门检查 `--json` 的 stdout 里没有原始控制字符），展示给人的程序自行清洗
- **`--json`**：stdout 只有一份 JSON，进度和告警走 stderr；失败时 stdout 为空、退出码非 0（`update` 例外：输出 `{packs, failed, apply, committed, history}` 后以非 0 退出，更新已保存但随后链接或 `--commit` 失败时同样输出，并多一个 `error` 字段；`outdated` 同理输出 `{checked, packs, failed, updates}`，`packs[]` 为 `{repo, commit, latest, changed, moved, new, gone, pending, checked}`）
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
