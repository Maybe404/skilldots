package cli

import (
	"strings"

	"github.com/spf13/cobra"
)

// The texts below stand in for the ones cobra generates for its
// `completion` command, which are English and cobra's own. They are used
// only with Chinese selected; English keeps cobra's, unchanged.

const (
	zhCompletionLong = "为指定的 shell 生成 skd 的自动补全脚本。\n各子命令的帮助说明了如何使用生成的脚本。\n"
	zhBashLong       = `为 bash 生成自动补全脚本。

这个脚本依赖 'bash-completion' 包。
如果还没有安装，可以用你系统的包管理器安装。

在当前 shell 会话中加载补全：

	source <(skd completion bash)

要让每个新会话都加载补全，请执行一次：

#### Linux：

	skd completion bash > /etc/bash_completion.d/skd

#### macOS：

	skd completion bash > $(brew --prefix)/etc/bash_completion.d/skd

需要开启一个新的 shell，这项设置才会生效。
`
	zhZshLong = `为 zsh 生成自动补全脚本。

如果你的环境里还没有启用 shell 补全，需要先启用它。可以执行一次：

	echo "autoload -U compinit; compinit" >> ~/.zshrc

在当前 shell 会话中加载补全：

	source <(skd completion zsh)

要让每个新会话都加载补全，请执行一次：

#### Linux：

	skd completion zsh > "${fpath[1]}/_skd"

#### macOS：

	skd completion zsh > $(brew --prefix)/share/zsh/site-functions/_skd

需要开启一个新的 shell，这项设置才会生效。
`
	zhFishLong = `为 fish 生成自动补全脚本。

在当前 shell 会话中加载补全：

	skd completion fish | source

要让每个新会话都加载补全，请执行一次：

	skd completion fish > ~/.config/fish/completions/skd.fish

需要开启一个新的 shell，这项设置才会生效。
`
	zhPowershellLong = `为 powershell 生成自动补全脚本。

在当前 shell 会话中加载补全：

	skd completion powershell | Out-String | Invoke-Expression

要让每个新会话都加载补全，请把上面命令的输出
加入你的 powershell 配置文件。
`
)

func localizeCompletion(root *cobra.Command) {
	var comp *cobra.Command
	for _, c := range root.Commands() {
		if c.Name() == "completion" {
			comp = c
		}
	}
	if comp == nil {
		return
	}
	comp.Short = "为指定的 shell 生成自动补全脚本"
	comp.Long = strings.Replace(zhCompletionLong, "skd", root.Name(), 1)
	longs := map[string]string{"bash": zhBashLong, "zsh": zhZshLong, "fish": zhFishLong, "powershell": zhPowershellLong}
	for _, sub := range comp.Commands() {
		if long, ok := longs[sub.Name()]; ok {
			sub.Short = "为 " + sub.Name() + " 生成自动补全脚本"
			sub.Long = long
			if f := sub.Flags().Lookup("no-descriptions"); f != nil {
				f.Usage = "不显示补全项的描述"
			}
		}
	}
}
