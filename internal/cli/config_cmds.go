package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Maybe404/skilldots/internal/config"
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
		Short: "Show or change your settings (editor, theme, lang)",
		Long: "Settings are per user and per machine, in " + config.Path() + ". " +
			"An empty value unsets a setting. lang is reserved: only English exists so far. " +
			"set changes only that setting's line: your comments and other lines stay (a value written over several " +
			"lines is the exception: then the file is written again, without comments).",
	}
	list := &cobra.Command{
		Use:   "list",
		Short: "Every setting with its value",
		Args:  cobra.NoArgs,
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
					v = dim("unset")
				}
				note := r.Help
				if r.ReadOnly != "" {
					note += " · set with " + r.ReadOnly
				}
				t.Row(r.Key, v, dim(note))
			}
			say("%s\n%s\n", t.Render(), dim(config.Path()))
			return nil
		},
	}
	get := &cobra.Command{
		Use:   "get <key>",
		Short: "Print one setting (empty when unset)",
		Args:  cobra.ExactArgs(1),
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
		Short: "Change a setting; without a value it is unset",
		Args:  cobra.RangeArgs(1, 2),
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
