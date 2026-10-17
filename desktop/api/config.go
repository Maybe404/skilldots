package api

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/Maybe404/skilldots/internal/config"
	"github.com/Maybe404/skilldots/internal/fsutil"
)

type preferences struct {
	CheckOnLaunch bool            `json:"checkOnLaunch"`
	HiddenAgents  map[string]bool `json:"hiddenAgents,omitempty"`
}

func readPreferences() (preferences, error) {
	p := preferences{CheckOnLaunch: true}
	b, err := os.ReadFile(prefPath())
	if os.IsNotExist(err) {
		return p, nil
	}
	if err != nil {
		return p, err
	}
	err = json.Unmarshal(b, &p)
	return p, err
}

func prefPath() string { return filepath.Join(filepath.Dir(config.Path()), "desktop.json") }
func readConfig() (object, error) {
	all, err := config.Load()
	if err != nil {
		return nil, err
	}
	prefs := preferences{CheckOnLaunch: true}
	if b, e := os.ReadFile(prefPath()); e == nil {
		if e = json.Unmarshal(b, &prefs); e != nil {
			return nil, e
		}
	} else if !os.IsNotExist(e) {
		return nil, e
	}
	lang := all[config.Lang]
	if lang != "zh" && lang != "en" {
		lang = "en"
	}
	theme := all[config.Theme]
	if theme == "" || theme == "auto" {
		theme = "system"
	}
	editor := all[config.Editor]
	kind := editor
	if editor == "" {
		kind = "auto"
	} else if editor != "code" && editor != "cursor" && editor != "zed" {
		kind = "custom"
	}
	return object{"editor": kind, "editorCommand": editor, "lang": lang, "theme": theme, "checkOnLaunch": prefs.CheckOnLaunch}, nil
}
func writeConfig(patch map[string]json.RawMessage) (object, error) {
	c, err := readConfig()
	if err != nil {
		return nil, err
	}
	for k, v := range patch {
		switch k {
		case "editor", "editorCommand", "lang", "theme":
			var str string
			if err = json.Unmarshal(v, &str); err != nil {
				return nil, err
			}
			c[k] = str
		case "checkOnLaunch":
			var b bool
			if err = json.Unmarshal(v, &b); err != nil {
				return nil, err
			}
			c[k] = b
		default:
			return nil, errors.New("unknown desktop setting")
		}
	}
	if c["editor"] != "auto" && c["editor"] != "code" && c["editor"] != "cursor" && c["editor"] != "zed" && c["editor"] != "custom" {
		return nil, errors.New("unknown editor")
	}
	values := map[string]string{config.Lang: c["lang"].(string), config.Theme: c["theme"].(string)}
	if values[config.Theme] == "system" {
		values[config.Theme] = "auto"
	}
	ed := c["editor"].(string)
	if ed == "auto" {
		ed = ""
	} else if ed == "custom" {
		ed = c["editorCommand"].(string)
	}
	values[config.Editor] = ed
	for k, v := range values {
		key, err := config.Lookup(k)
		if err != nil {
			return nil, err
		}
		if err = key.Check(v); err != nil {
			return nil, err
		}
	}
	for k, v := range values {
		if err = config.Set(k, v); err != nil {
			return nil, err
		}
	}
	prefs, err := readPreferences()
	if err != nil {
		return nil, err
	}
	prefs.CheckOnLaunch = c["checkOnLaunch"].(bool)
	b, err := json.Marshal(prefs)
	if err != nil {
		return nil, err
	}
	if err = fsutil.WriteAtomic(prefPath(), b); err != nil {
		return nil, err
	}
	return readConfig()
}
