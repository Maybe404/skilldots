//go:build desktop_smoke

// Run only with an isolated HOME and SKILLDOTS_SMOKE_HOME pointing to it.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Maybe404/skilldots/desktop/api"
	"github.com/egoist/mygo"
)

func main() {
	home, _ := os.UserHomeDir()
	if home == "" || home != os.Getenv("SKILLDOTS_SMOKE_HOME") || len(os.Args) != 3 {
		panic("isolated HOME and frontend/report paths required")
	}
	s := api.New("", api.Native{})
	invoke := func(name string, args ...any) {
		raw := []json.RawMessage{}
		for _, a := range args {
			b, _ := json.Marshal(a)
			raw = append(raw, b)
		}
		if _, err := s.Invoke(context.Background(), name, raw); err != nil {
			panic(err)
		}
	}
	invoke("initLibrary", filepath.Join(home, "library"))
	invoke("setConfig", map[string]any{"lang": "zh", "checkOnLaunch": false})
	invoke("newSkill", map[string]any{"name": "native-smoke", "description": "原生窗口真实后端验证", "agents": []string{"agents", "claude"}})
	mygo.Bind(s)
	mygo.SetFrontend(os.DirFS(os.Args[1]))
	var failure error
	mygo.App.WhenReady(func() {
		win := mygo.NewWindow(mygo.WindowOptions{Title: "skilldots integration test", URL: "/", Width: 1280, Height: 820, Vibrancy: mygo.VibrancyUnderWindow})
		go func() {
			defer mygo.App.Quit()
			time.Sleep(time.Second)
			result, err := win.Page().Eval(`
     const delay = ms => new Promise(r => setTimeout(r, ms));
     const invoke = (method, ...args) => window.mygo.call("Service.Invoke", method, args);
     const waitFor = async fn => { for (let i=0;i<150;i++) { const v=await fn(); if(v) return v; await delay(100); } throw new Error("timeout: "+document.body.innerText); };
     await waitFor(() => document.body.innerText.includes("native-smoke"));
     const button = () => [...document.querySelectorAll("button[aria-pressed]")].find(b => b.getAttribute("aria-label")?.includes("native-smoke") && b.getAttribute("aria-label")?.includes("claude"));
     const footer = () => document.querySelector("footer")?.innerText ?? "";
     const applyButton = () => [...document.querySelectorAll("footer button")].find(b => b.textContent.trim() === "应用");
     const claudeLinked = async () => (await invoke("skills"))[0].targets.includes("claude");
     // A toggle only records a pending change; the links change when 应用 is pressed.
     const flip = async wantOn => {
       const toggle = await waitFor(button);
       toggle.click();
       await waitFor(() => button()?.getAttribute("aria-pressed") === String(wantOn) && footer().includes("1 处改动待应用"));
       if ((await claudeLinked()) === wantOn) throw new Error("toggle changed the links before Apply");
       (await waitFor(applyButton)).click();
       await waitFor(async () => (await claudeLinked()) === wantOn);
       await waitFor(() => button() && !button().disabled && button().getAttribute("aria-pressed") === String(wantOn) && !footer().includes("待应用"));
     };
     await waitFor(button);
     await flip(false);
     await flip(true);
     const ref = {key:"skills:native-smoke"};
     const text = await invoke("readFile",ref,"SKILL.md");
     await invoke("writeFile",ref,"SKILL.md",text+"\nNative bridge edit.\n");
     if (!(await invoke("readFile",ref,"SKILL.md")).includes("Native bridge edit.")) throw new Error("edit failed");
     const plan = await invoke("plan");
     if (plan.length) throw new Error("links did not settle: "+JSON.stringify(plan));
     return {ok:true,platform:window.mygo.platform,font:getComputedStyle(document.body).fontFamily,pingFang:document.fonts.check('16px "PingFang SC"'),blur:CSS.supports("backdrop-filter","blur(24px)"),text:document.body.innerText};
   `)
			report := map[string]any{"result": result}
			if err != nil {
				report["error"] = err.Error()
				failure = err
			}
			b, _ := json.MarshalIndent(report, "", "  ")
			if e := os.WriteFile(os.Args[2], b, 0600); e != nil {
				failure = e
			}
			if png, e := win.CapturePage(); e == nil {
				os.WriteFile(os.Args[2]+".png", png, 0600)
			}
		}()
	})
	if err := mygo.App.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if failure != nil {
		fmt.Fprintln(os.Stderr, failure)
		os.Exit(1)
	}
}
