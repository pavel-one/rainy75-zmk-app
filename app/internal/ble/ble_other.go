//go:build !darwin

// Package ble — транспорт SMP по Bluetooth LE; сейчас есть только для macOS.
package ble

import (
	"errors"
	"time"
)

const Supported = false

type Port struct{}

func Open(string, time.Duration) (*Port, error) {
	return nil, errors.New("управление по Bluetooth пока есть только на macOS")
}

func (*Port) Write([]byte) (int, error)          { return 0, errors.New("не поддерживается") }
func (*Port) Read([]byte) (int, error)           { return 0, errors.New("не поддерживается") }
func (*Port) SetReadTimeout(time.Duration) error { return nil }
func (*Port) Close() error                       { return nil }
