package main

import (
	"context"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"rainy75/internal/i18n"
	"rainy75/internal/update"
)

const (
	eventUpdate         = "update:available"
	eventUpdateProgress = "update:progress"
	checkEvery          = 6 * time.Hour
)

type UpdateInfo struct {
	Current   string         `json:"current"`
	Available bool           `json:"available"`
	Release   update.Release `json:"release"`
}

type UpdateProgress struct {
	Done  int64 `json:"done"`
	Total int64 `json:"total"`
}

type updater struct {
	mu         sync.Mutex
	last       UpdateInfo
	installing bool
}

func (s *KeyboardService) Version() string { return version }

// CheckUpdate спрашивает GitHub о последнем релизе.
func (s *KeyboardService) CheckUpdate() (UpdateInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	r, err := update.Latest(ctx)
	if err != nil {
		return UpdateInfo{Current: version}, err
	}
	info := UpdateInfo{Current: version, Available: update.Newer(r.Version, version), Release: r}
	s.upd.mu.Lock()
	s.upd.last = info
	s.upd.mu.Unlock()
	return info, nil
}

// InstallUpdate скачивает и ставит последний релиз, затем перезапускает приложение.
func (s *KeyboardService) InstallUpdate() error {
	s.upd.mu.Lock()
	info := s.upd.last
	if s.upd.installing {
		s.upd.mu.Unlock()
		return i18n.Error("обновление уже устанавливается")
	}
	s.upd.installing = true
	s.upd.mu.Unlock()
	defer func() {
		s.upd.mu.Lock()
		s.upd.installing = false
		s.upd.mu.Unlock()
	}()

	if !info.Available {
		var err error
		if info, err = s.CheckUpdate(); err != nil {
			return err
		}
		if !info.Available {
			return i18n.Error("установлена последняя версия")
		}
	}
	var lastPct int64 = -1
	err := update.Install(context.Background(), info.Release, func(done, total int64) {
		if total > 0 && done*100/total != lastPct {
			lastPct = done * 100 / total
			emit(eventUpdateProgress, UpdateProgress{Done: done, Total: total})
		}
	})
	if err != nil {
		return err
	}
	if app := application.Get(); app != nil {
		go func() {
			time.Sleep(300 * time.Millisecond)
			app.Quit()
		}()
	}
	return nil
}

func (s *KeyboardService) watchUpdates(ctx context.Context) {
	update.Cleanup()
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		if info, err := s.CheckUpdate(); err == nil && info.Available {
			emit(eventUpdate, info)
		}
		timer.Reset(checkEvery)
	}
}
