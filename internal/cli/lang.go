package cli

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Maybe404/skilldots/internal/config"
	"github.com/Maybe404/skilldots/internal/i18n"
)

// scanArgs reads, before cobra has parsed anything, the two global flags
// that decide the language of the command tree itself (its help is built
// before a command runs): --lang and --json. Arguments after "--" are
// not flags.
func scanArgs(args []string) (lang string, json bool) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--":
			return lang, json
		case a == "--lang":
			if i+1 < len(args) {
				lang = args[i+1]
				i++
			}
		case strings.HasPrefix(a, "--lang="):
			lang = strings.TrimPrefix(a, "--lang=")
		case a == "--json":
			json = true
		case strings.HasPrefix(a, "--json="):
			json = strings.TrimPrefix(a, "--json=") != "false" && strings.TrimPrefix(a, "--json=") != "0"
		}
	}
	return lang, json
}

// langSettingWarned is set once a bad lang setting was reported.
var langSettingWarned bool

// applyLang chooses the language: --lang, $SKILLDOTS_LANG, the lang
// setting, the system's; and with --json, English. What --json prints
// must not depend on the language, and the same goes for the messages
// that travel in it (reasons, problems, errors), so under --json every
// message is English. The values it passed over as not a language are
// returned.
func applyLang(flag string, json bool) (bad []i18n.Source) {
	if json {
		i18n.Set(i18n.EN)
		return nil
	}
	l, _, bad := i18n.Resolve(i18n.Inputs{Flag: flag, Setting: config.Get(config.Lang)})
	i18n.Set(l)
	return bad
}

// setupLang is applyLang as the command's first step, once cobra has
// parsed the flags: it refuses a --lang or $SKILLDOTS_LANG that is not a
// language (as a bad --theme is refused), and warns about a bad setting,
// which must not stop every command, `skd config set lang` included.
func setupLang() error {
	for _, src := range applyLang(flags.lang, flags.json) {
		switch src {
		case i18n.FromFlag:
			return i18n.Errorf("%s %q: use auto, en or zh", src, flags.lang)
		case i18n.FromEnv:
			return i18n.Errorf("%s %q: use auto, en or zh", src, os.Getenv(i18n.Env))
		case i18n.FromSetting:
			if !langSettingWarned {
				langSettingWarned = true
				i18n.Fprintf(stderr, "warning: the lang setting %q is not auto, en or zh; using auto (skd config set lang auto fixes it)\n", config.Get(config.Lang))
			}
		}
	}
	return nil
}

// label puts a status label ("warning:", "error:") before text: a space
// between them, except after the full-width colon of Chinese, which
// carries its own.
func label(label string, text string) string {
	return tag(func(s string) string { return s }, label, text)
}

// tag is label with the label drawn in a style.
func tag(style func(string) string, label string, text string) string {
	if strings.HasSuffix(label, "：") {
		return style(label) + text
	}
	return style(label) + " " + text
}

// pad pads s with spaces on the right to w columns. English pads by
// characters, as fmt's %-ns always did here; Chinese by terminal cells,
// two for a Chinese character, so columns line up.
func pad(s string, w int) string {
	if i18n.Get() == i18n.EN {
		return fmt.Sprintf("%-*s", w, s)
	}
	return i18n.PadRight(s, w)
}

// localizeCobra gives the parts of the command tree that cobra writes
// itself (its usage template, the help flags, the help command and the
// completion commands) the language in use. English is cobra's own text,
// untouched.
func localizeCobra(root *cobra.Command) {
	if i18n.Get() != i18n.ZH {
		return
	}
	root.SetUsageTemplate(strings.NewReplacer(
		"Usage:", "用法：",
		"Aliases:", "别名：",
		"Examples:", "示例：",
		"Available Commands:", "可用命令：",
		"Additional Commands:", "其他命令：",
		"Global Flags:", "全局选项：",
		"Flags:", "选项：",
		"Additional help topics:", "其他帮助主题：",
		`Use "{{.CommandPath}} [command] --help" for more information about a command.`,
		`使用 "{{.CommandPath}} [command] --help" 查看某个命令的更多信息。`,
	).Replace(root.UsageTemplate()))
	root.SetVersionTemplate("{{with .Name}}{{printf \"%s \" .}}{{end}}{{printf \"版本 %s\" .Version}}\n")
	root.InitDefaultHelpCmd()
	root.InitDefaultCompletionCmd()
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		name := c.Name()
		if c.Flags().Lookup("help") == nil {
			c.Flags().BoolP("help", "h", false, i18n.Sprintf("help for %s", name))
		}
		if c == root && c.Flags().Lookup("version") == nil && c.Version != "" {
			c.Flags().BoolP("version", "v", false, i18n.Sprintf("version for %s", name))
		}
		switch c.Name() {
		case "help":
			if c.Parent() == root {
				c.Short = i18n.T("Help about any command")
				c.Long = i18n.Sprintf("Help provides help for any command in the application.\nSimply type %s help [path to command] for full details.", root.Name())
			}
		}
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(root)
	localizeCompletion(root)
}

// The argument checks of cobra, with its messages, translated. English is
// cobra's text, unchanged.

func exactArgs(n int) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) != n {
			return i18n.Errorf("accepts %d arg(s), received %d", n, len(args))
		}
		return nil
	}
}

func rangeArgs(min, max int) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) < min || len(args) > max {
			return i18n.Errorf("accepts between %d and %d arg(s), received %d", min, max, len(args))
		}
		return nil
	}
}

func maximumNArgs(n int) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) > n {
			return i18n.Errorf("accepts at most %d arg(s), received %d", n, len(args))
		}
		return nil
	}
}

func noArgs(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		return i18n.Errorf("unknown command %q for %q", args[0], cmd.CommandPath())
	}
	return nil
}

// rootArgs is what cobra checks of the arguments of a command that has
// subcommands and none of its own: the first is not a subcommand.
func rootArgs(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return nil
	}
	msg := i18n.Sprintf("unknown command %q for %q", args[0], cmd.CommandPath())
	if cmd.SuggestionsMinimumDistance <= 0 {
		cmd.SuggestionsMinimumDistance = 2
	}
	if suggestions := cmd.SuggestionsFor(args[0]); !cmd.DisableSuggestions && len(suggestions) > 0 {
		msg += i18n.T("\n\nDid you mean this?\n")
		for _, s := range suggestions {
			msg += "\t" + s + "\n"
		}
	}
	return errors.New(msg) // i18n:raw (already translated)
}

var flagErrors = []struct {
	re     *regexp.Regexp
	format string
}{
	{regexp.MustCompile(`^unknown flag: (.*)$`), i18n.M("unknown flag: %s")},
	{regexp.MustCompile(`^unknown shorthand flag: (.*) in -(.*)$`), i18n.M("unknown shorthand flag: %s in -%s")},
	{regexp.MustCompile(`^flag needs an argument: (.*) in -(.*)$`), i18n.M("flag needs an argument: %s in -%s")},
	{regexp.MustCompile(`^flag needs an argument: (.*)$`), i18n.M("flag needs an argument: %s")},
	{regexp.MustCompile(`^invalid argument (".*") for (".*") flag: (.*)$`), i18n.M("invalid argument %s for %s flag: %s")},
}

// flagError gives the errors pflag reports for a command line the
// language in use.
func flagError(cmd *cobra.Command, err error) error {
	if i18n.Get() == i18n.EN {
		return err
	}
	for _, fe := range flagErrors {
		if m := fe.re.FindStringSubmatch(err.Error()); m != nil {
			args := make([]any, len(m)-1)
			for i, s := range m[1:] {
				args[i] = s
			}
			return i18n.Errorf(fe.format, args...)
		}
	}
	return err
}
