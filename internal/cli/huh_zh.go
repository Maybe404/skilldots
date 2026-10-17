package cli

import (
	"reflect"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/huh"

	"github.com/Maybe404/skilldots/internal/i18n"
)

// zhKeyHelp is what the key hints under a prompt (huh's own, English) say
// in Chinese.
var zhKeyHelp = map[string]string{
	"filter": "过滤", "set filter": "确定过滤", "clear filter": "清除过滤", "first": "开头", "last": "末尾",
	"go to start": "到开头", "go to end": "到末尾", "new line": "换行", "select all": "全选", "select none": "全不选",
	"½ page up": "向上半页", "½ page down": "向下半页", "page up": "上一页", "page down": "下一页",
	"complete": "补全", "open editor": "打开编辑器", "confirm": "确认", "next": "下一步", "back": "返回",
	"open": "打开", "close": "关闭", "select": "选择", "submit": "提交", "toggle": "切换",
	"left": "左", "right": "右", "up": "上", "down": "下", "Yes": "是", "No": "否",
}

// keyMap is huh's key map with the hints in the language in use.
func keyMap() *huh.KeyMap {
	km := huh.NewDefaultKeyMap()
	if i18n.Get() == i18n.ZH {
		localizeBindings(reflect.ValueOf(km).Elem())
	}
	return km
}

func localizeBindings(v reflect.Value) {
	switch v.Kind() {
	case reflect.Struct:
		if v.Type() == reflect.TypeOf(key.Binding{}) {
			b := v.Interface().(key.Binding)
			if h := b.Help(); h.Desc != "" {
				if zh, ok := zhKeyHelp[h.Desc]; ok {
					b.SetHelp(h.Key, zh)
					v.Set(reflect.ValueOf(b))
				}
			}
			return
		}
		for i := 0; i < v.NumField(); i++ {
			localizeBindings(v.Field(i))
		}
	}
}
