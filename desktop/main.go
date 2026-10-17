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
		mygo.NewWindow(mygo.WindowOptions{Title: "skilldots", URL: "/", Width: 1280, Height: 820, Vibrancy: mygo.VibrancyUnderWindow})
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
