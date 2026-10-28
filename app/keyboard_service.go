package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"rainy75/internal/i18n"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"rainy75/internal/autostart"
	"rainy75/internal/battery"
	"rainy75/internal/ble"
	"rainy75/internal/device"
	"rainy75/internal/effects"
	"rainy75/internal/layout"
	"rainy75/internal/lights"
	"rainy75/internal/rainy"
	"rainy75/internal/settings"
	"rainy75/internal/smp"
)

const (
	eventDevice    = "device:status"
	eventBattery   = "battery:status"
	eventProgress  = "firmware:progress"
	eventLighting  = "lighting:state"
	eventFrame     = "lighting:frame" // кадр для схемы в интерфейсе; null — эффекты прошивки
	eventAutostart = "autostart:changed"
	eventPomodoro  = "pomodoro:state"
	eventLanguage  = "language:changed"

	pollDevice  = 2 * time.Second
	pollBattery = 30 * time.Second

	// Подключение по Bluetooth дольше и заметнее, поэтому пробуем реже.
	bleRetry   = 10 * time.Second
	bleTimeout = 6 * time.Second
	bleName    = "Rainy"
)

type DeviceStatus struct {
	Connected  bool   `json:"connected"`
	Connection string `json:"connection"` // usb | bluetooth
	Port       string `json:"port"`
	Firmware   string `json:"firmware"`
	Windows    bool   `json:"windows"`
	Host       bool   `json:"host"` // подсветкой управляет компьютер
	Message    string `json:"message"`
}

type Progress struct {
	Stage string `json:"stage"` // upload | reboot | done | error
	Done  int    `json:"done"`
	Total int    `json:"total"`
	Text  string `json:"text"`
}

type FirmwareFile struct {
	Path    string `json:"path"`
	Name    string `json:"name"`
	Version string `json:"version"`
	Size    int    `json:"size"`
}

type FirmwareInfo struct {
	Connected bool          `json:"connected"`
	Images    []rainy.Image `json:"images"`
}

func init() {
	application.RegisterEvent[DeviceStatus](eventDevice)
	application.RegisterEvent[battery.Status](eventBattery)
	application.RegisterEvent[Progress](eventProgress)
	application.RegisterEvent[LightingState](eventLighting)
	application.RegisterEvent[[]string](eventFrame)
	application.RegisterEvent[bool](eventAutostart)
	application.RegisterEvent[lights.PomodoroState](eventPomodoro)
	application.RegisterEvent[string](eventLanguage)
	application.RegisterEvent[UpdateInfo](eventUpdate)
	application.RegisterEvent[UpdateProgress](eventUpdateProgress)
}

type KeyboardService struct {
	mu       sync.Mutex
	client   *smp.Client
	kb       *rainy.Keyboard
	status   DeviceStatus
	flashing bool // идёт обновление — канал занят загрузкой
	bleTried time.Time
	cancel   context.CancelFunc

	cfgMu   sync.Mutex
	cfg     settings.Settings
	cfgPath string

	windows atomic.Bool // режим для сопоставления нажатий с клавишами
	lighting
	upd updater
}

func NewKeyboardService() *KeyboardService {
	s := &KeyboardService{status: DeviceStatus{Message: device.ErrNotFound.Error()}, cfg: settings.Default()}
	if path, err := settings.Path(); err == nil {
		s.cfgPath = path
		s.cfg = settings.Load(path)
	}
	i18n.Set(i18n.Resolve(s.cfg.Language))
	s.status.Message = device.ErrNotFound.Error()
	s.initLighting()
	return s
}

func (s *KeyboardService) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	ctx, s.cancel = context.WithCancel(ctx)
	go s.watchDevice(ctx)
	go s.watchBattery(ctx)
	go s.watchUpdates(ctx)
	s.startLighting(ctx)
	return nil
}

func (s *KeyboardService) ServiceShutdown() error {
	if s.cancel != nil {
		s.cancel()
	}
	s.stopLighting()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.disconnectLocked("")
	return nil
}

func emit(name string, data any) {
	if app := application.Get(); app != nil {
		app.Event.Emit(name, data)
	}
}

func (s *KeyboardService) config() settings.Settings {
	s.cfgMu.Lock()
	defer s.cfgMu.Unlock()
	return s.cfg
}

func (s *KeyboardService) watchDevice(ctx context.Context) {
	t := time.NewTicker(pollDevice)
	defer t.Stop()
	for {
		s.pollDevice()
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (s *KeyboardService) pollDevice() {
	s.mu.Lock()
	before := s.status
	switch {
	case s.flashing:
	case s.kb == nil:
		if s.connectLocked() {
			s.engine.Resync()
		}
	default:
		info, err := s.kb.Info()
		if err != nil {
			s.disconnectLocked(i18n.T("Связь с клавиатурой потеряна"))
			break
		}
		// Прошивка сама вышла из режима кадров — нажали Fn-клавишу подсветки.
		// Под той же блокировкой, что и отправка кадров, чтобы не спутать с ней.
		if !info.Host {
			s.engine.HostLost()
		}
		// Режим сменился не из приложения — значит, нажали Fn+M.
		if info.Windows != s.status.Windows {
			if cfg := s.config().Indicator; cfg.Enabled && cfg.OnHardware {
				s.mode.Trigger(cfg, info.Windows, time.Now())
			}
		}
		s.status.Windows, s.status.Host = info.Windows, info.Host
		s.windows.Store(info.Windows)
	}
	after := s.status
	s.mu.Unlock()
	if after != before {
		emit(eventDevice, after)
	}
}

// connectLocked ищет клавиатуру: сначала кабель, затем Bluetooth.
func (s *KeyboardService) connectLocked() bool {
	if s.flashing {
		return false
	}
	msg := device.ErrNotFound.Error()
	ports, err := device.Find()
	if err != nil {
		msg = err.Error()
	}
	for _, p := range ports {
		c, err := device.Open(p)
		if err != nil {
			msg = err.Error()
			continue
		}
		if s.attachLocked(c, "usb", p.Port) {
			return true
		}
		msg = i18n.T("на порту не отвечает прошивка rainy75-studio (группа mcumgr 65)")
	}

	if ble.Supported && s.config().Bluetooth && time.Since(s.bleTried) >= bleRetry {
		s.bleTried = time.Now()
		port, err := ble.Open(bleName, bleTimeout)
		if err == nil {
			c := smp.NewRawClient(port)
			c.Timeout = 4 * time.Second
			if s.attachLocked(c, "bluetooth", "Bluetooth") {
				return true
			}
			msg = i18n.T("клавиатура по Bluetooth не отвечает на команды SMP")
		} else if len(ports) == 0 {
			msg = i18n.Sprintf("Rainy75 не найдена: подключите кабель USB-C или Bluetooth (%v)", err)
		}
	}
	s.status = DeviceStatus{Message: msg}
	return false
}

// attachLocked проверяет, что на канале отвечает нужная прошивка, и запоминает его.
func (s *KeyboardService) attachLocked(c *smp.Client, conn, port string) bool {
	kb := rainy.New(c)
	// Первая команда сразу после загрузки прошивки иногда теряется — повторяем.
	info, err := kb.Info()
	if errors.Is(err, smp.ErrTimeout) {
		info, err = kb.Info()
	}
	if err != nil {
		c.Close()
		return false
	}
	st := DeviceStatus{Connected: true, Connection: conn, Port: port, Firmware: i18n.T("нет данных"), Windows: info.Windows, Host: info.Host}
	if imgs, err := kb.Images(); err == nil {
		for _, img := range imgs {
			if img.Active {
				st.Firmware = img.Version
			}
		}
	}
	s.client, s.kb, s.status = c, kb, st
	s.windows.Store(info.Windows)
	return true
}

func (s *KeyboardService) disconnectLocked(msg string) {
	if s.client != nil {
		s.client.Close()
		s.client, s.kb = nil, nil
	}
	if msg == "" {
		msg = device.ErrNotFound.Error()
	}
	s.status = DeviceStatus{Message: msg}
}

// withKeyboard выполняет fn с подключённой клавиатурой; при сбое связи отключается.
// Сам не подключается: поиск идёт в фоне, чтобы вызовы не ждали Bluetooth.
func (s *KeyboardService) withKeyboard(fn func(kb *rainy.Keyboard) error) error {
	s.mu.Lock()
	if s.flashing {
		s.mu.Unlock()
		return i18n.Error("идёт обновление прошивки")
	}
	if s.kb == nil {
		err := errors.New(s.status.Message)
		s.mu.Unlock()
		return err
	}
	err := fn(s.kb)
	lost := errors.Is(err, smp.ErrIO) || errors.Is(err, smp.ErrTimeout)
	if lost {
		s.disconnectLocked(i18n.T("Связь с клавиатурой потеряна"))
	}
	st := s.status
	s.mu.Unlock()
	if lost {
		emit(eventDevice, st)
	}
	return err
}

func (s *KeyboardService) Status() DeviceStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

func (s *KeyboardService) Layout() layout.Layout { return layout.Rainy75 }

func (s *KeyboardService) Effects() []effects.Effect {
	out := make([]effects.Effect, len(effects.List))
	for i, e := range effects.List {
		e.Name = i18n.T(e.Name)
		out[i] = e
	}
	return out
}

// Language возвращает язык интерфейса: выбранный или язык системы.
func (s *KeyboardService) Language() string { return i18n.Lang() }

func modeName(win bool) string {
	if win {
		return "windows"
	}
	return "mac"
}

func (s *KeyboardService) OSMode() (string, error) {
	var win bool
	err := s.withKeyboard(func(kb *rainy.Keyboard) (err error) {
		win, err = kb.Windows()
		return err
	})
	return modeName(win), err
}

// SetOSMode переключает режим (как Fn+M) и возвращает итоговый.
func (s *KeyboardService) SetOSMode(mode string) (string, error) {
	if mode != "windows" && mode != "mac" {
		return "", i18n.Errorf("неизвестный режим %q", mode)
	}
	var win bool
	err := s.withKeyboard(func(kb *rainy.Keyboard) (err error) {
		win, err = kb.SetWindows(mode == "windows")
		if err == nil {
			// Под той же блокировкой, что и опрос: иначе он примет смену за Fn+M.
			s.status.Windows = win
			s.windows.Store(win)
		}
		return err
	})
	if err == nil {
		emit(eventDevice, s.Status())
		if cfg := s.config().Indicator; cfg.Enabled {
			s.mode.Trigger(cfg, win, time.Now())
		}
	}
	return modeName(win), err
}

func (s *KeyboardService) watchBattery(ctx context.Context) {
	t := time.NewTicker(pollBattery)
	defer t.Stop()
	for {
		emit(eventBattery, battery.Read(ctx))
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// Battery возвращает заряд: прошивка публикует его по Bluetooth, ОС его читает.
func (s *KeyboardService) Battery(ctx context.Context) battery.Status {
	return battery.Read(ctx)
}

func (s *KeyboardService) Autostart() bool { return autostart.Enabled() }

func (s *KeyboardService) SetAutostart(on bool) (bool, error) {
	err := autostart.Set(on)
	now := autostart.Enabled()
	emit(eventAutostart, now)
	return now, err
}

func (s *KeyboardService) FirmwareInfo() (FirmwareInfo, error) {
	var info FirmwareInfo
	err := s.withKeyboard(func(kb *rainy.Keyboard) (err error) {
		info.Images, err = kb.Images()
		return err
	})
	info.Connected = err == nil
	return info, err
}

func readImage(path string) ([]byte, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	ver, err := rainy.ImageVersion(data)
	return data, ver, err
}

func (s *KeyboardService) ChooseFirmware() (FirmwareFile, error) {
	app := application.Get()
	if app == nil {
		return FirmwareFile{}, i18n.Error("приложение не запущено")
	}
	path, err := app.Dialog.OpenFile().
		SetTitle(i18n.T("Образ прошивки Rainy75 (zmk.signed.bin)")).
		AddFilter(i18n.T("Образ MCUboot"), "*.bin").
		PromptForSingleSelection()
	if err != nil || path == "" {
		return FirmwareFile{}, err
	}
	data, ver, err := readImage(path)
	if err != nil {
		return FirmwareFile{}, err
	}
	return FirmwareFile{Path: path, Name: filepath.Base(path), Version: ver, Size: len(data)}, nil
}

// FlashFirmware загружает образ во второй слот, помечает его для пробного
// запуска и перезагружает клавиатуру. MCUboot вернёт прежнюю прошивку, если
// новая не запустится.
func (s *KeyboardService) FlashFirmware(path string) error {
	data, _, err := readImage(path)
	if err != nil {
		return err
	}
	s.mu.Lock()
	kb := s.kb
	if kb == nil {
		msg := s.status.Message
		s.mu.Unlock()
		return errors.New(msg)
	}
	s.flashing = true // подсветка и опрос ждут: канал занят загрузкой
	s.mu.Unlock()

	finish := func(msg string) {
		s.mu.Lock()
		s.flashing = false
		s.disconnectLocked(msg)
		st := s.status
		s.mu.Unlock()
		emit(eventDevice, st)
	}
	fail := func(err error) error {
		emit(eventProgress, Progress{Stage: "error", Text: err.Error()})
		finish(i18n.Sprintf("Обновление прервано: %v", err))
		return err
	}

	err = kb.Upload(data, func(done, total int) {
		emit(eventProgress, Progress{Stage: "upload", Done: done, Total: total, Text: i18n.T("Загрузка образа")})
	})
	if err != nil {
		return fail(err)
	}
	imgs, err := kb.Images()
	if err != nil {
		return fail(err)
	}
	var hash []byte
	for _, img := range imgs {
		if img.Slot == 1 {
			hash = img.Hash
		}
	}
	if hash == nil {
		return fail(i18n.Error("образ не появился во втором слоте"))
	}
	if err := kb.Test(hash); err != nil {
		return fail(err)
	}
	emit(eventProgress, Progress{Stage: "reboot", Done: len(data), Total: len(data), Text: i18n.T("Перезагрузка: MCUboot ставит новую прошивку")})
	_ = kb.Reset() // клавиатура может не успеть ответить до перезагрузки

	finish(i18n.T("Клавиатура перезагружается с новой прошивкой…"))
	emit(eventProgress, Progress{Stage: "done", Done: len(data), Total: len(data), Text: i18n.T("Готово")})
	return nil
}
