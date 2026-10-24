//go:build !darwin && !linux

package battery

import "context"

func readPlatform(context.Context) Status {
	s := notFound()
	s.Message = "На этой ОС заряд не публикуется. Нажмите Fn+B — индикатор на клавишах покажет уровень."
	return s
}
