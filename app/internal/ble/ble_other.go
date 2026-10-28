//go:build !darwin

// Package ble — транспорт SMP по Bluetooth LE; сейчас есть только для macOS.
package ble

import (
	"rainy75/internal/i18n"
	"time"
)

const Supported = false

type Port struct{}

func Open(string, time.Duration) (*Port, error) {
	return nil, i18n.Error("управление по Bluetooth пока есть только на macOS")
}

func (*Port) Write([]byte) (int, error)          { return 0, i18n.Error("не поддерживается") }
func (*Port) Read([]byte) (int, error)           { return 0, i18n.Error("не поддерживается") }
func (*Port) SetReadTimeout(time.Duration) error { return nil }
func (*Port) MaxWrite() int                      { return 0 }
func (*Port) Close() error                       { return nil }
