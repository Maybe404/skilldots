package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Maybe404/skilldots/internal/config"
	"github.com/Maybe404/skilldots/internal/i18n"
)

// settingRow is one setting as `skd config list --json` prints it.
type settingRow struct {
	Key      string   `json:"key"`
	Value    string   `json:"value"`
	Default  string   `json:"default"`
	Values   []string `json:"values,omitempty"`
	Help     string   `json:"help"`
	ReadOnly string   `json:"readOnly,omitempty"`
}

func configCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: i18n.T("Show or change your settings (editor, theme, lang)"),
		Long: i18n.Sprintf("Settings are per user and per machine, in %s. "+
			"An empty value unsets a setting. lang is the language of skd's own text: en, zh or auto (the system's). "+
			"set changes only that setting's line: your comments and other lines stay, and so does a comment at the end of "+
			"that line (unsetting removes the line with it; a value written over several lines is the exception: then the "+
			"file is written again, without comments). Lines end like the file's (LF or CRLF).", config.Path()),
	}
	list := &cobra.Command{
		Use:   "list",
		Short: i18n.T("Every setting with its value"),
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			all, err := config.Load()
			if err != nil {
				return err
			}
			var rows []settingRow
			for _, k := range config.Keys {
				rows = append(rows, settingRow{Key: k.Name, Value: all[k.Name], Default: k.Default, Values: k.Values, Help: k.Help, ReadOnly: k.ReadOnly})
			}
			if jsonOut(rows) {
				return nil
			}
			t := newTable("SETTING", "VALUE", "")
			for _, r := range rows {
				v := txt(r.Value)
				if v == "" {
					v = dim(i18n.T("unset"))
				}
				note := i18n.T(r.Help)
				if r.ReadOnly != "" {
					note += i18n.Sprintf(" · set with %s", r.ReadOnly)
				}
				t.Row(r.Key, v, dim(note))
			}
			say("%s\n%s\n", t.Render(), dim(config.Path()))
			return nil
		},
	}
	get := &cobra.Command{
		Use:   "get <key>",
		Short: i18n.T("Print one setting (empty when unset)"),
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := config.Lookup(args[0]); err != nil {
				return err
			}
			all, err := config.Load()
			if err != nil {
				return err
			}
			if !jsonOut(map[string]string{"key": args[0], "value": all[args[0]]}) {
				fmt.Fprintln(stdout, txt(all[args[0]]))
			}
			return nil
		},
	}
	set := &cobra.Command{
		Use:   "set <key> [value]",
		Short: i18n.T("Change a setting; without a value it is unset"),
		Args:  rangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			value := ""
			if len(args) == 2 {
				value = args[1]
			}
			if err := config.Set(args[0], value); err != nil {
				return err
			}
			if value == "" {
				say("%s %s unset\n", ok("✓"), args[0])
			} else {
				say("%s %s = %s\n", ok("✓"), args[0], txt(value))
			}
			jsonOut(map[string]string{"key": args[0], "value": value})
			return nil
		},
	}
	cmd.AddCommand(list, get, set)
	return cmd
}
