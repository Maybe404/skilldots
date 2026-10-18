package main

import (
	"context"
	"log"
	"os"

	"github.com/Maybe404/skilldots/desktop/api"
	"github.com/egoist/mygo"
)

func main() {
	service := api.New(os.Getenv("SKILLDOTS_LIBRARY"), api.Native{
		Reveal: func(path string) error { mygo.Shell.ShowItemInFolder(path); return nil },
		PickFolder: func(ctx context.Context) (string, error) {
			paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: mygo.CallerWindow(ctx), Directory: true})
			if err != nil || len(paths) == 0 {
				return "", err
			}
			return paths[0], nil
		},
	})
	mygo.Bind(service)
	mygo.App.WhenReady(func() {
		mygo.NewWindow(mygo.WindowOptions{
			Title:  "skilldots",
			URL:    "/",
			Width:  1280,
			Height: 820,
			// Below this the top bar no longer fits (English labels, with the
			// traffic lights' inset); dialogs still fit at this height.
			MinWidth:  860,
			MinHeight: 560,
			Vibrancy:  mygo.VibrancyUnderWindow,
			// No native title bar: the app's top bar is the only row at the
			// top. The traffic lights sit at its left end, centred on the line
			// of its controls (54px bar, 14px buttons: 20px above and below).
			// The page keeps clear of them with the --mygo-titlebar-* CSS
			// variables and drags the window by the bar (--app-region: drag).
			// Keep in step with cmd/smoke.
			TitleBarStyle:        mygo.TitleBarHidden,
			TrafficLightPosition: &mygo.Point{X: 20, Y: 20},
		})
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
