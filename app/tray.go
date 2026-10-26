package main

import (
	_ "embed"
	"runtime"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"rainy75/internal/lights"
)

// Шаблон для строки меню macOS (система сама красит его под тему) и цветная
// иконка для Windows и Linux.
var (
	//go:embed build/tray/tray-template.png
	trayTemplateIcon []byte
	//go:embed build/tray/tray-color.png
	trayColorIcon []byte
)

func modeTitle(win bool) string {
	if win {
		return "Windows"
	}
	return "macOS"
}

// setupTray создаёт значок в трее: режим Win/Mac, эффекты прошивки,
// автозапуск, открытие окна и выход.
func setupTray(app *application.App, window *application.WebviewWindow, kb *KeyboardService) {
	tray := app.SystemTray.New()
	if runtime.GOOS == "darwin" {
		tray.SetTemplateIcon(trayTemplateIcon)
	} else {
		tray.SetIcon(trayColorIcon)
	}
	tray.SetTooltip("Rainy75")

	showWindow := func() {
		window.Show()
		window.Restore()
		window.Focus()
	}

	var renderNow func(DeviceStatus)
	menu := application.NewMenu()
	status := menu.Add("Поиск клавиатуры…").SetEnabled(false)
	menu.AddSeparator()
	mac := menu.AddRadio("macOS", false)
	win := menu.AddRadio("Windows", false)
	menu.AddSeparator()
	restore := menu.Add("Вернуть эффекты прошивки")
	pomodoro := menu.Add("Помодоро: старт")
	menu.AddSeparator()
	auto := menu.AddCheckbox("Запускать при входе в систему", kb.Autostart())
	menu.Add("Открыть Rainy75").OnClick(func(*application.Context) { showWindow() })
	menu.AddSeparator()
	menu.Add("Выйти").OnClick(func(*application.Context) { app.Quit() })

	// Нативное меню трогаем только из главного потока и без пересборки
	// (menu.Update пересоздаёт пункты): каждый пункт обновляется сам.
	render := func(st DeviceStatus) { application.InvokeSync(func() { renderNow(st) }) }
	renderNow = func(st DeviceStatus) {
		if st.Connected {
			status.SetLabel("Rainy75 подключена · " + modeTitle(st.Windows))
			tray.SetTooltip("Rainy75 — режим " + modeTitle(st.Windows))
		} else {
			status.SetLabel("Клавиатура не подключена по USB")
			tray.SetTooltip("Rainy75 — не подключена")
		}
		// В строке меню macOS рядом со значком виден текущий режим.
		if runtime.GOOS == "darwin" {
			label := ""
			if st.Connected {
				label = map[bool]string{true: "Win", false: "Mac"}[st.Windows]
			}
			tray.SetLabel(label)
		}
		mac.SetChecked(st.Connected && !st.Windows)
		win.SetChecked(st.Connected && st.Windows)
		for _, item := range []*application.MenuItem{mac, win, restore} {
			item.SetEnabled(st.Connected)
		}
	}

	setMode := func(mode string) {
		go func() {
			if _, err := kb.SetOSMode(mode); err != nil {
				app.Logger.Error("трей: переключение режима", "error", err)
			}
			render(kb.Status()) // вернуть отметку, если не получилось
		}()
	}
	mac.OnClick(func(*application.Context) { setMode("mac") })
	win.OnClick(func(*application.Context) { setMode("windows") })
	restore.OnClick(func(*application.Context) {
		go func() {
			if err := kb.RestoreEffects(); err != nil {
				app.Logger.Error("трей: эффекты прошивки", "error", err)
			}
		}()
	})
	auto.OnClick(func(ctx *application.Context) {
		on, err := kb.SetAutostart(ctx.ClickedMenuItem().Checked())
		if err != nil {
			app.Logger.Error("трей: автозапуск", "error", err)
		}
		application.InvokeSync(func() { auto.SetChecked(on) })
	})

	app.Event.On(eventDevice, func(e *application.CustomEvent) {
		if st, ok := e.Data.(DeviceStatus); ok {
			render(st)
		}
	})
	renderPomodoro := func(st lights.PomodoroState) {
		label := "Помодоро: старт"
		switch {
		case st.Running && st.Phase == "work":
			label = "Помодоро: пауза (работа)"
		case st.Running:
			label = "Помодоро: пауза (отдых)"
		case st.Phase != "idle":
			label = "Помодоро: продолжить"
		}
		application.InvokeSync(func() { pomodoro.SetLabel(label) })
	}
	pomodoro.OnClick(func(*application.Context) {
		if st := kb.PomodoroState(); st.Running {
			kb.PomodoroPause()
		} else {
			kb.PomodoroStart()
		}
	})
	app.Event.On(eventPomodoro, func(e *application.CustomEvent) {
		if st, ok := e.Data.(lights.PomodoroState); ok {
			renderPomodoro(st)
		}
	})

	app.Event.On(eventAutostart, func(e *application.CustomEvent) {
		if on, ok := e.Data.(bool); ok {
			application.InvokeSync(func() { auto.SetChecked(on) })
		}
	})

	tray.SetMenu(menu)
	// Начальное состояние — когда приложение запущено и меню можно обновлять.
	app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		go render(kb.Status())
	})
}
