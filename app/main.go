package main

import (
	"embed"
	"log"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"rainy75/internal/autostart"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	var window *application.WebviewWindow
	kb := NewKeyboardService()
	// При автозапуске приложение стартует в трее, без окна.
	hidden := slices.Contains(os.Args[1:], autostart.HiddenFlag)
	app := application.New(application.Options{
		Name:        "Rainy75",
		Description: "Панель управления Rainy75 с открытой прошивкой ZMK",
		// Порт клавиатуры открывает только одна программа, поэтому второй
		// запуск лишь показывает окно — или, с --flash, мигает клавиатурой.
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: "one.pavel.rainy75",
			OnSecondInstanceLaunch: func(data application.SecondInstanceData) {
				if flash, ok := parseFlash(data.Args); ok {
					if err := kb.Flash(flash.color, flash.times); err != nil {
						log.Printf("вспышка: %v", err)
					}
					return
				}
				if window != nil {
					window.Restore()
					window.Show()
					window.Focus()
				}
			},
		},
		Services: []application.Service{
			application.NewService(kb),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			// Закрытое окно прячется: приложение продолжает работать в трее.
			ApplicationShouldTerminateAfterLastWindowClosed: false,
		},
	})

	window = app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:     "Rainy75",
		Width:     1180,
		Height:    780,
		MinWidth:  960,
		MinHeight: 640,
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 52,
			Backdrop:                application.MacBackdropTranslucent,
			TitleBar:                application.MacTitleBarHiddenInset,
		},
		BackgroundColour: application.NewRGB(11, 12, 20),
		URL:              "/",
		Hidden:           hidden,
	})

	window.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		window.Hide()
		e.Cancel()
	})
	// Клик по значку в Dock возвращает спрятанное окно.
	app.Event.OnApplicationEvent(events.Mac.ApplicationShouldHandleReopen, func(*application.ApplicationEvent) {
		window.Show()
		window.Focus()
	})
	setupTray(app, window, kb)

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

type flashArgs struct {
	color string
	times int
}

// parseFlash разбирает `--flash [#rrggbb] [--times N]`: вспышка по команде,
// например из скрипта, когда нужно привлечь внимание.
func parseFlash(args []string) (flashArgs, bool) {
	f := flashArgs{color: "#ffffff", times: 3}
	found := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--flash":
			found = true
			if i+1 < len(args) && strings.HasPrefix(args[i+1], "#") {
				f.color = args[i+1]
				i++
			}
		case "--times":
			if i+1 < len(args) {
				if n, err := strconv.Atoi(args[i+1]); err == nil {
					f.times = n
				}
				i++
			}
		}
	}
	return f, found
}
