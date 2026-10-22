package main

// Встраиваем Info.plist в бинарник: macOS читает из него тексты разрешений
// (Bluetooth, звук системы), даже когда приложение запущено не из .app.

/*
#cgo LDFLAGS: -Wl,-sectcreate,__TEXT,__info_plist,${SRCDIR}/build/darwin/Info.embedded.plist
*/
import "C"
