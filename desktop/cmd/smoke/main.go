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
	// The machine has Claude Code: the toggles below link into ~/.claude/skills,
	// and skd links nothing into an agent that is not installed.
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		panic(err)
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
     // The wire view's agent switch (aria-checked); the table view's cells are toggle buttons (aria-pressed).
     const button = () => [...document.querySelectorAll("button[aria-pressed], [role=switch]")].find(b => b.getAttribute("aria-label")?.includes("native-smoke") && b.getAttribute("aria-label")?.includes("claude"));
     const isOn = b => (b.getAttribute("aria-pressed") ?? b.getAttribute("aria-checked")) === "true";
     const footer = () => document.querySelector("footer")?.innerText ?? "";
     const applyButton = () => [...document.querySelectorAll("footer button")].find(b => b.textContent.trim() === "应用");
     const claudeLinked = async () => (await invoke("skills"))[0].targets.includes("claude");
     // A toggle only records a pending change; the links change when 应用 is pressed.
     const flip = async wantOn => {
       const toggle = await waitFor(button);
       toggle.click();
       await waitFor(() => button() && isOn(button()) === wantOn && footer().includes("1 处改动待应用"));
       if ((await claudeLinked()) === wantOn) throw new Error("toggle changed the links before Apply");
       (await waitFor(applyButton)).click();
       await waitFor(async () => (await claudeLinked()) === wantOn);
       await waitFor(() => button() && !button().disabled && isOn(button()) === wantOn && !footer().includes("待应用"));
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
     const titleBar = await checkBar("start");
     ` + titleBarSwitches + `
     return {ok:true,platform:window.mygo.platform,font:getComputedStyle(document.body).fontFamily,pingFang:document.fonts.check('16px "PingFang SC"'),blur:CSS.supports("backdrop-filter","blur(24px)"),titleBar,switches,text:document.body.innerText};
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

// titleBarCheck (JavaScript) defines checkBar(what), which checks that the top
// bar is the window's title bar: mygo reports the room of the traffic lights,
// the brand starts right of them, the bar drags the window while its controls
// do not, and the controls share the traffic lights' centre line. It leaves
// the numbers in the object it returns.
//
// It measures only a settled bar. After a reload, a style, page or language
// switch the layout is not final yet (the old document, fonts still loading,
// the highlight thumb springing), so it samples the bar's geometry every 50ms
// and takes it when it has not changed for 300ms and the fonts are loaded.
// firstNavMid (the first sample) and settleMs show how far off it was before.
const titleBarCheck = `
     const checkBar = async what => {
       const wait = ms => new Promise(r => setTimeout(r, ms));
       const box = el => el.getBoundingClientRect();
       const mid = el => { const r = box(el); return r.top + r.height / 2; };
       const t0 = performance.now();
       // window.__smokeStale is set just before a reload: the old document must be gone first.
       while (window.__smokeStale || !document.querySelector("header nav")) {
         if (performance.now() - t0 > 15000) throw new Error(what + ": no top bar: " + document.body.innerText);
         await wait(50);
       }
       const header = document.querySelector("header");
       const nav = header.querySelector("nav"), add = [...header.querySelectorAll("button")].at(-1), search = header.querySelector("input");
       const geometry = () => [header, nav, add, search].map(el => { const r = box(el); return [r.top, r.height].map(n => n.toFixed(2)).join(","); }).join("|") + "|" + document.fonts.status;
       const first = mid(nav);
       const series = [];
       let key = geometry(), since = performance.now();
       for (;;) {
         await wait(50);
         const now = geometry();
         if (now !== key) { key = now; since = performance.now(); series.push([Math.round(since - t0), mid(nav)]); }
         if (performance.now() - since >= 300 && document.fonts.status === "loaded") break;
         if (performance.now() - t0 > 8000) throw new Error(what + ": the top bar never settled: " + JSON.stringify(series));
       }
       const root = getComputedStyle(document.documentElement);
       const inset = parseFloat(root.getPropertyValue("--mygo-titlebar-inset-left")) || 0;
       const height = parseFloat(root.getPropertyValue("--mygo-titlebar-height")) || 0;
       const region = el => getComputedStyle(el).getPropertyValue("--app-region").trim();
       const titleBar = {what, style: document.documentElement.dataset.style, inset, height, fullScreen: inset === 0, brandLeft: box(header.firstElementChild).left, paddingLeft: getComputedStyle(header).paddingLeft,
         headerRegion: region(header), navRegion: region(nav), searchRegion: region(header.querySelector("[data-slot=input-group]")),
         lightsMid: height / 2, navMid: mid(nav), addMid: mid(add), searchMid: mid(search), firstNavMid: first, settleMs: Math.round(since - t0)};
       if (!titleBar.fullScreen) {
         if (titleBar.inset < 40 || titleBar.height < 30) throw new Error(what + ": no room reported for the traffic lights: " + JSON.stringify(titleBar));
         if (titleBar.brandLeft < titleBar.inset + 8) throw new Error(what + ": the brand sits under the traffic lights: " + JSON.stringify(titleBar));
         for (const k of ["navMid", "addMid", "searchMid"]) if (Math.abs(titleBar[k] - titleBar.lightsMid) > 1) throw new Error(what + ": " + k + " is off the traffic lights' centre line: " + JSON.stringify(titleBar));
       }
       if (titleBar.headerRegion !== "drag" || titleBar.navRegion !== "no-drag" || titleBar.searchRegion !== "no-drag") throw new Error(what + ": drag regions: " + JSON.stringify(titleBar));
       return titleBar;
     };
`

// titleBarSwitches (JavaScript) switches the style, the pages and the
// language through the interface, in the window, and checks the bar after
// each switch (see titleBarCheck). It leaves the style, language and page it
// started in, and the checks' numbers in switches.
const titleBarSwitches = `
     const switches = [];
     const pause = ms => new Promise(r => setTimeout(r, ms));
     const headerButton = name => [...document.querySelectorAll("header button, header [role=radio]")].find(b => b.textContent.trim() === name);
     const press = name => async () => { const b = headerButton(name); if (!b) throw new Error("no button " + name + " in the top bar"); b.click(); };
     // The style switch lives in Settings (not in the top bar): open it, then pick.
     const style = name => async () => {
       const settings = headerButton("设置") || headerButton("Settings");
       if (!settings) throw new Error("no Settings in the top bar");
       settings.click();
       await pause(400);
       const b = [...document.querySelectorAll("button, [role=radio]")].find(b => !b.closest("header") && b.textContent.trim() === name);
       if (!b) throw new Error("no style " + name + " in Settings");
       b.click();
     };
     const pickLanguage = label => async () => {
       document.querySelector("button[aria-label='语言'], button[aria-label='Language']").click();
       await pause(300);
       [...document.querySelectorAll("[role=option]")].find(o => o.textContent.trim() === label).click();
     };
     const startStyle = document.documentElement.dataset.style === "doodle" ? "涂鸦" : "简洁";
     const barSteps = [
       ["style: doodle", style("涂鸦")], ["style: clean", style("简洁")], ["style: doodle again", style("涂鸦")], ["style: clean again", style("简洁")],
       ["page: sources", press("来源")], ["page: agents", press("Agent")], ["page: skills", press("Skill")], ["page: settings", press("设置")],
       ["language: en", pickLanguage("English")],
       ["en style: doodle", style("Doodle")], ["en style: clean", style("Clean")], ["en page: sources", press("Sources")], ["en page: skills", press("Skills")], ["en page: settings", press("Settings")],
       ["language: zh", pickLanguage("简体中文")], ["restore style", style(startStyle)], ["zh page: skills", press("Skill")],
     ];
     for (const [what, act] of barSteps) {
       await act();
       const m = await checkBar(what);
       switches.push({what, style: m.style, navMid: m.navMid, firstNavMid: m.firstNavMid, settleMs: m.settleMs});
     }
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
		if os.Getenv("SKILLDOTS_SMOKE_CAPTURE") == "page" {
			return nil // only the <name>.page.png taken inside the webview
		}
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
	measure := func(name string) (any, error) {
		return win.Page().Eval(titleBarCheck + `return await checkBar(` + fmt.Sprintf("%q", name) + `);`)
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
	// A second skill, linked to one agent only, so the wire view has an "adding" wire to show.
	if _, err := win.Page().Eval(`await window.mygo.call("Service.Invoke", "newSkill", [{name: "partial-skill", description: "linked to one agent", agents: ["agents"]}]); return true;`); err != nil {
		return err
	}
	states := []struct{ name, style, lang, js string }{
		{"native-clean-zh", "clean", "zh", ""}, {"native-doodle-zh", "doodle", "zh", ""}, {"native-clean-en", "clean", "en", ""},
		// Wire view in every state: hover (focus) on one skill, a pending removal and a pending addition.
		{"native-clean-wires", "clean", "zh", wireStates}, {"native-doodle-wires", "doodle", "zh", wireStates},
		{"native-clean-table", "clean", "zh", tableState}, {"native-doodle-table", "doodle", "zh", tableState},
		// The rest of the doodle style: other pages, a dialog and the detail sheet.
		{"native-doodle-sources", "doodle", "zh", pageState("来源")}, {"native-doodle-agents", "doodle", "zh", pageState("Agent")},
		{"native-doodle-settings", "doodle", "zh", pageState("设置")}, {"native-doodle-add", "doodle", "zh", addDialogState},
		{"native-doodle-detail", "doodle", "zh", detailState},
	}
	for _, st := range states {
		if _, err := win.Page().Eval(fmt.Sprintf(`localStorage.setItem("skilldots.desktop.style", %q);
     await window.mygo.call("Service.Invoke", "setConfig", [{lang: %q}]);
     window.__smokeStale = true;
     setTimeout(() => location.reload(), 50);
     return true;`, st.style, st.lang)); err != nil {
			return err
		}
		time.Sleep(1500 * time.Millisecond)
		if st.js != "" {
			// The state's own steps need the new document up.
			if _, err := win.Page().Eval(`for (let i = 0; i < 150 && (window.__smokeStale || !document.querySelector("header nav")); i++) await new Promise(r => setTimeout(r, 100)); return true;`); err != nil {
				return fmt.Errorf("%s: %w", st.name, err)
			}
			if _, err := win.Page().Eval(st.js); err != nil {
				return fmt.Errorf("%s: %w", st.name, err)
			}
			time.Sleep(900 * time.Millisecond)
		}
		// Every state's bar is checked once it has settled, whatever the state did to the page.
		m, err := measure(st.name)
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
	m, err := measure("native-fullscreen")
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

const uiHelpers = `
     const delay = ms => new Promise(r => setTimeout(r, ms));
     const byText = (sel, text) => [...document.querySelectorAll(sel)].find(b => b.textContent.trim() === text);
     const row = name => [...document.querySelectorAll("[id^=skill-row-]")].find(b => b.getAttribute("aria-label")?.startsWith(name));
     // A click on the selected row clears the selection, so only an unselected row is clicked.
     const pick = name => row(name).getAttribute("aria-current") === "true" || row(name).click();
     const toggle = agent => document.querySelector("button[aria-pressed][aria-label*='(" + agent + ")'], [role=switch][aria-label*='(" + agent + ")']");
`

// wireStates: native-smoke is hovered (its wires turn orange and thick), its
// link to claude is pending removal (grey dots), partial-skill gets a pending
// link to claude (orange dashes) and is the selected skill.
const wireStates = uiHelpers + `
     pick("native-smoke"); await delay(150);
     toggle("claude").click(); await delay(150);
     pick("partial-skill"); await delay(150);
     toggle("claude").click(); await delay(150);
     row("native-smoke").parentElement.dispatchEvent(new MouseEvent("mouseover", {bubbles: true}));
     await delay(300);
     return true;`

const tableState = uiHelpers + `
     byText("button", "表格").click(); await delay(500);
     return true;`

func pageState(label string) string {
	return uiHelpers + `
     byText("header nav button", "` + label + `").click(); await delay(700);
     return true;`
}

const addDialogState = uiHelpers + `
     [...document.querySelectorAll("header button")].at(-1).click(); await delay(900);
     return true;`

const detailState = uiHelpers + `
     row("native-smoke").dispatchEvent(new MouseEvent("dblclick", {bubbles: true})); await delay(1000);
     return true;`
