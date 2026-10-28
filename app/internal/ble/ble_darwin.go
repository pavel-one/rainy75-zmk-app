// Package ble — транспорт SMP (mcumgr) по Bluetooth LE через CoreBluetooth:
// управление клавиатурой без кабеля, когда она подключена к Mac по Bluetooth.
package ble

/*
#cgo CFLAGS: -x objective-c -fobjc-arc -mmacosx-version-min=12.0
#cgo LDFLAGS: -framework Foundation -framework CoreBluetooth
#include <stdlib.h>
#include "ble_darwin.h"
*/
import "C"

import (
	"errors"
	"rainy75/internal/i18n"
	"sync"
	"time"
	"unsafe"
)

const Supported = true

var ErrClosed = i18n.Error("соединение по Bluetooth закрыто")

// Port — SMP-канал по Bluetooth; совместим с smp.Port.
type Port struct {
	timeout time.Duration
}

// Одно соединение на приложение: делегат CoreBluetooth общий.
var (
	mu     sync.Mutex
	cond   = sync.NewCond(&mu)
	buf    []byte
	closed = true
)

//export rbGoData
func rbGoData(data unsafe.Pointer, n C.int) {
	b := C.GoBytes(data, n)
	mu.Lock()
	buf = append(buf, b...)
	mu.Unlock()
	cond.Broadcast()
}

//export rbGoDisconnected
func rbGoDisconnected() {
	mu.Lock()
	closed = true
	mu.Unlock()
	cond.Broadcast()
}

func Open(name string, timeout time.Duration) (*Port, error) {
	cname := C.CString(name)
	defer C.free(unsafe.Pointer(cname))
	if rc := C.rb_connect(cname, C.int(timeout.Milliseconds())); rc != 0 {
		return nil, errors.New(C.GoString(C.rb_error(rc)))
	}
	mu.Lock()
	buf, closed = nil, false
	mu.Unlock()
	return &Port{timeout: time.Second}, nil
}

func (p *Port) Write(b []byte) (int, error) {
	if len(b) == 0 {
		return 0, nil
	}
	if rc := C.rb_write(unsafe.Pointer(&b[0]), C.int(len(b))); rc != 0 {
		return 0, errors.New(C.GoString(C.rb_error(rc)))
	}
	return len(b), nil
}

// Read ждёт данные не дольше таймаута; по истечении возвращает 0 байт.
func (p *Port) Read(b []byte) (int, error) {
	deadline := time.Now().Add(p.timeout)
	mu.Lock()
	defer mu.Unlock()
	for len(buf) == 0 && !closed {
		left := time.Until(deadline)
		if left <= 0 {
			return 0, nil
		}
		// sync.Cond не умеет ждать с таймаутом — будим себя таймером.
		t := time.AfterFunc(left, cond.Broadcast)
		cond.Wait()
		t.Stop()
	}
	if len(buf) == 0 {
		return 0, ErrClosed
	}
	n := copy(b, buf)
	buf = buf[n:]
	return n, nil
}

// MaxWrite — сколько байт помещается в одну запись. Пакет длиннее прошивка
// собирает из частей, и под потоком кадров эта сборка сбивается — поэтому
// частые команды стоит укладывать в одну запись.
func (p *Port) MaxWrite() int { return int(C.rb_max_write()) }

func (p *Port) SetReadTimeout(t time.Duration) error {
	p.timeout = t
	return nil
}

func (p *Port) Close() error {
	C.rb_disconnect()
	rbGoDisconnected()
	return nil
}
