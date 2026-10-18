//go:build desktop_smoke

// Run only with an isolated HOME and SKILLDOTS_SMOKE_HOME pointing to it.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
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
		// The window options of desktop/main.go: no native title bar, traffic lights over the top bar.
		win := mygo.NewWindow(mygo.WindowOptions{
			Title: "skilldots integration test", URL: "/", Width: 1280, Height: 820, Vibrancy: mygo.VibrancyUnderWindow,
			TitleBarStyle: mygo.TitleBarHidden, TrafficLightPosition: &mygo.Point{X: 20, Y: 20}, MinWidth: 860, MinHeight: 560,
		})
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
     ` + titleBarCheck + `
     return {ok:true,platform:window.mygo.platform,font:getComputedStyle(document.body).fontFamily,pingFang:document.fonts.check('16px "PingFang SC"'),blur:CSS.supports("backdrop-filter","blur(24px)"),titleBar,text:document.body.innerText};
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
			// Optional: screenshots of the whole window, traffic lights included.
			if dir := os.Getenv("SKILLDOTS_SMOKE_SHOTS"); dir != "" && failure == nil {
				if e := nativeShots(win, dir); e != nil {
					failure = e
				}
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

// titleBarCheck (JavaScript) checks that the top bar is the window's title
// bar: mygo reports the room of the traffic lights, the brand starts right of
// them, the bar drags the window while its controls do not, and the controls
// share the traffic lights' centre line. It leaves the numbers in titleBar.
const titleBarCheck = `
     const titleBar = (() => {
       const root = getComputedStyle(document.documentElement);
       const inset = parseFloat(root.getPropertyValue("--mygo-titlebar-inset-left")) || 0;
       const height = parseFloat(root.getPropertyValue("--mygo-titlebar-height")) || 0;
       const header = document.querySelector("header");
       const region = el => getComputedStyle(el).getPropertyValue("--app-region").trim();
       const box = el => el.getBoundingClientRect();
       const mid = el => { const r = box(el); return r.top + r.height / 2; };
       const nav = header.querySelector("nav"), add = [...header.querySelectorAll("button")].at(-1);
       return {inset, height, fullScreen: inset === 0, brandLeft: box(header.firstElementChild).left, paddingLeft: getComputedStyle(header).paddingLeft,
         headerRegion: region(header), navRegion: region(nav), searchRegion: region(header.querySelector("[data-slot=input-group]")),
         lightsMid: height / 2, navMid: mid(nav), addMid: mid(add), searchMid: mid(header.querySelector("input"))};
     })();
     if (!titleBar.fullScreen) {
       if (titleBar.inset < 40 || titleBar.height < 30) throw new Error("no room reported for the traffic lights: " + JSON.stringify(titleBar));
       if (titleBar.brandLeft < titleBar.inset + 8) throw new Error("the brand sits under the traffic lights: " + JSON.stringify(titleBar));
       for (const k of ["navMid", "addMid", "searchMid"]) if (Math.abs(titleBar[k] - titleBar.lightsMid) > 1.5) throw new Error(k + " is off the traffic lights' centre line: " + JSON.stringify(titleBar));
     }
     if (titleBar.headerRegion !== "drag" || titleBar.navRegion !== "no-drag" || titleBar.searchRegion !== "no-drag") throw new Error("drag regions: " + JSON.stringify(titleBar));
`

// nativeShots captures the whole window (traffic lights included) in both
// styles and in English, then in full screen, with screencapture. That needs
// the Screen Recording permission; macOS may refuse it to a process started
// by this harness, so with SKILLDOTS_SMOKE_CAPTURE=external the harness only
// writes <name>.rect ("x,y,w,h") and waits up to 30s for <name>.png, which a
// script outside (one that has the permission) takes with
// `screencapture -x -o -R <rect> <name>.png`. Each state's title bar numbers
// go to titlebar.json next to the images.
func nativeShots(win *mygo.Window, dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	win.SetAlwaysOnTop(true)
	win.Focus()
	time.Sleep(700 * time.Millisecond)
	capture := func(name string) error {
		b := win.Bounds()
		rect := fmt.Sprintf("%d,%d,%d,%d", b.X, b.Y, b.Width, b.Height)
		png := filepath.Join(dir, name+".png")
		if os.Getenv("SKILLDOTS_SMOKE_CAPTURE") != "external" {
			if out, err := exec.Command("screencapture", "-x", "-o", "-R", rect, png).CombinedOutput(); err != nil {
				return fmt.Errorf("screencapture %s %s: %v %s", name, rect, err, out)
			}
			return nil
		}
		if err := os.WriteFile(filepath.Join(dir, name+".rect"), []byte(rect), 0o600); err != nil {
			return err
		}
		for i := 0; i < 300; i++ {
			if st, err := os.Stat(png); err == nil && st.Size() > 0 {
				time.Sleep(300 * time.Millisecond)
				return nil
			}
			time.Sleep(100 * time.Millisecond)
		}
		return fmt.Errorf("no screenshot for %s (%s) within 30s", name, rect)
	}
	measure := func() (any, error) {
		return win.Page().Eval(`
     const delay = ms => new Promise(r => setTimeout(r, ms));
     for (let i = 0; i < 100 && !document.querySelector("header nav"); i++) await delay(100);
     await delay(600);
     ` + titleBarCheck + `
     return titleBar;`)
	}
	metrics := map[string]any{}
	// A screenshot that cannot be taken (a locked screen, no permission) is
	// reported, not fatal: the checks above already ran. The page alone
	// (without the window frame) is always saved as <name>.page.png.
	var captureErrors []string
	shot := func(name string) {
		if png, err := win.CapturePage(); err == nil {
			os.WriteFile(filepath.Join(dir, name+".page.png"), png, 0o600)
		}
		if err := capture(name); err != nil {
			captureErrors = append(captureErrors, err.Error())
			fmt.Fprintln(os.Stderr, "warning:", err)
		}
	}
	states := []struct{ name, style, lang string }{{"native-clean-zh", "clean", "zh"}, {"native-doodle-zh", "doodle", "zh"}, {"native-clean-en", "clean", "en"}}
	for _, st := range states {
		if _, err := win.Page().Eval(fmt.Sprintf(`localStorage.setItem("skilldots.desktop.style", %q);
     await window.mygo.call("Service.Invoke", "setConfig", [{lang: %q}]);
     setTimeout(() => location.reload(), 50);
     return true;`, st.style, st.lang)); err != nil {
			return err
		}
		time.Sleep(1500 * time.Millisecond)
		m, err := measure()
		if err != nil {
			return fmt.Errorf("%s: %w", st.name, err)
		}
		metrics[st.name] = m
		shot(st.name)
	}
	// A double click on the bar does what the system setting says (zoom by default).
	before := win.Bounds()
	if _, err := win.Page().Eval(`const s = document.querySelector("header > .flex-1"); s.dispatchEvent(new MouseEvent("mousedown", {bubbles: true, button: 0, detail: 2})); return true;`); err != nil {
		return err
	}
	time.Sleep(1200 * time.Millisecond)
	zoomed := win.Bounds()
	metrics["doubleClick"] = map[string]any{"before": before, "after": zoomed, "changed": zoomed != before}
	if zoomed != before {
		win.SetBounds(before)
		time.Sleep(800 * time.Millisecond)
	}
	// Full screen hides the traffic lights: the bar gives their room back.
	win.SetAlwaysOnTop(false)
	win.SetFullScreen(true)
	time.Sleep(2500 * time.Millisecond)
	m, err := measure()
	if err != nil {
		return fmt.Errorf("full screen: %w", err)
	}
	metrics["native-fullscreen"] = m
	shot("native-fullscreen")
	win.SetFullScreen(false)
	time.Sleep(2000 * time.Millisecond)
	metrics["captureErrors"] = captureErrors
	b, _ := json.MarshalIndent(metrics, "", "  ")
	return os.WriteFile(filepath.Join(dir, "titlebar.json"), b, 0o600)
}
