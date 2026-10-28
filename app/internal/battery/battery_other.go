//go:build !darwin && !linux

package battery

import (
	"context"

	"rainy75/internal/i18n"
)

func readPlatform(context.Context) Status {
	s := notFound()
	s.Message = i18n.T("На этой ОС заряд не публикуется. Нажмите Fn+B — индикатор на клавишах покажет уровень.")
	return s
}
