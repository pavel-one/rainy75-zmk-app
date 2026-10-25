//go:build !darwin

package host

import "errors"

const Supported = false

var errUnsupported = errors.New("доступно только на macOS")

var ErrNoInputMonitoring = errUnsupported

func KeyTapAllowed() bool                     { return false }
func RequestKeyTap() bool                     { return false }
func StartKeyTap(func(code string)) error     { return errUnsupported }
func StopKeyTap()                             {}
func InputLanguage() string                   { return "" }
func CapsLock() bool                          { return false }
func Volume() (float64, bool, error)          { return 0, false, errUnsupported }
func StartAudio() error                       { return errUnsupported }
func StopAudio()                              {}
func AudioSamples(n int) ([]float32, float64) { return make([]float32, n), 48000 }

type CPU struct{}

// Usage — не поддерживается.
func (c *CPU) Usage() (float64, error) { return 0, errUnsupported }
