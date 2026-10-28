// Package rainy — команды открытой прошивки Rainy75 (rainy75-studio) по SMP:
// группа 65 (покадровая подсветка и режим Win/Mac) и стандартные группы
// mcumgr (образы MCUboot, перезагрузка).
package rainy

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"rainy75/internal/i18n"
	"time"

	"rainy75/internal/smp"
)

// Positions — сколько клавиш (позиций keymap) у Rainy75: ISO, построчно.
const Positions = 83

// Группа 65 (zmk/src/rgb_mgmt.c в rainy75-studio).
const (
	groupRGB = 65

	cmdSet   = 0
	cmdFill  = 1
	cmdClear = 2
	cmdInfo  = 3
	cmdFrame = 4
	cmdOS    = 5
)

const (
	cmdImageState  = 0
	cmdImageUpload = 1
	cmdOSReset     = 5
)

type Caller interface {
	Call(op byte, group uint16, id byte, req, resp any) error
	CallTimeout(timeout time.Duration, op byte, group uint16, id byte, req, resp any) error
}

type Keyboard struct{ c Caller }

func New(c Caller) *Keyboard { return &Keyboard{c: c} }

type Info struct {
	Keys    int  `cbor:"n" json:"keys"`
	Host    bool `cbor:"host" json:"host"` // подсветкой управляет компьютер
	Windows bool `cbor:"win" json:"windows"`
}

func (k *Keyboard) Info() (Info, error) {
	var i Info
	err := k.c.Call(smp.OpRead, groupRGB, cmdInfo, nil, &i)
	return i, err
}

type Color struct{ R, G, B uint8 }

func (k *Keyboard) Fill(c Color) error {
	return k.c.Call(smp.OpWrite, groupRGB, cmdFill, map[string]any{"r": c.R, "g": c.G, "b": c.B}, nil)
}

// SetKeys меняет цвет отдельных клавиш (позиция → цвет). Первый вызов после
// эффектов прошивки начинает с чёрного кадра.
func (k *Keyboard) SetKeys(colors map[int]Color) error {
	px := make([]byte, 0, 4*len(colors))
	for pos, c := range colors {
		if pos < 0 || pos >= Positions {
			return i18n.Errorf("нет клавиши %d", pos)
		}
		px = append(px, byte(pos), c.R, c.G, c.B)
	}
	if len(px) == 0 {
		return nil
	}
	return k.c.Call(smp.OpWrite, groupRGB, cmdSet, map[string]any{"px": px}, nil)
}

func (k *Keyboard) Frame(frame []Color) error {
	if len(frame) != Positions {
		return i18n.Errorf("в кадре должно быть %d цветов, а не %d", Positions, len(frame))
	}
	rgb := make([]byte, 0, 3*Positions)
	for _, c := range frame {
		rgb = append(rgb, c.R, c.G, c.B)
	}
	return k.c.Call(smp.OpWrite, groupRGB, cmdFrame, map[string]any{"rgb": rgb}, nil)
}

// Clear возвращает эффекты прошивки (выход из режима управления с компьютера).
func (k *Keyboard) Clear() error {
	return k.c.Call(smp.OpWrite, groupRGB, cmdClear, nil, nil)
}

// Windows сообщает, включён ли режим Windows (слой 2 поверх раскладки Mac).
func (k *Keyboard) Windows() (bool, error) {
	var r struct {
		Win bool `cbor:"win"`
	}
	err := k.c.Call(smp.OpRead, groupRGB, cmdOS, nil, &r)
	return r.Win, err
}

func (k *Keyboard) SetWindows(win bool) (bool, error) {
	var r struct {
		Win bool `cbor:"win"`
	}
	err := k.c.Call(smp.OpWrite, groupRGB, cmdOS, map[string]any{"win": win}, &r)
	return r.Win, err
}

type Image struct {
	Slot      int    `cbor:"slot" json:"slot"`
	Version   string `cbor:"version" json:"version"`
	Hash      []byte `cbor:"hash" json:"-"`
	Bootable  bool   `cbor:"bootable" json:"bootable"`
	Pending   bool   `cbor:"pending" json:"pending"`
	Confirmed bool   `cbor:"confirmed" json:"confirmed"`
	Active    bool   `cbor:"active" json:"active"`
	Permanent bool   `cbor:"permanent" json:"permanent"`
}

func (k *Keyboard) Images() ([]Image, error) {
	var r struct {
		Images []Image `cbor:"images"`
	}
	err := k.c.Call(smp.OpRead, smp.GroupImage, cmdImageState, nil, &r)
	return r.Images, err
}

// uploadChunk — данных в одном запросе: пакет целиком должен поместиться
// в буфер SMP прошивки (CONFIG_MCUMGR_TRANSPORT_NETBUF_SIZE=512).
const uploadChunk = 384

// Первый кусок ждёт стирания слота — это долго.
const uploadTimeout = 20 * time.Second

func (k *Keyboard) Upload(img []byte, progress func(done, total int)) error {
	sum := sha256.Sum256(img)
	off := 0
	for off < len(img) {
		end := min(off+uploadChunk, len(img))
		req := map[string]any{"off": off, "data": img[off:end]}
		if off == 0 {
			req["len"] = len(img)
			req["sha"] = sum[:]
		}
		var r struct {
			Off *int `cbor:"off"`
		}
		if err := k.c.CallTimeout(uploadTimeout, smp.OpWrite, smp.GroupImage, cmdImageUpload, req, &r); err != nil {
			return i18n.Errorf("загрузка, смещение %d: %w", off, err)
		}
		if r.Off == nil || *r.Off < off || *r.Off > len(img) {
			return i18n.Errorf("загрузка, смещение %d: неожиданный ответ клавиатуры", off)
		}
		off = *r.Off
		if progress != nil {
			progress(off, len(img))
		}
	}
	return nil
}

// Test помечает образ для пробного запуска: MCUboot поставит его при
// следующей загрузке и вернёт прежний, если новый не подтвердится.
func (k *Keyboard) Test(hash []byte) error {
	return k.c.Call(smp.OpWrite, smp.GroupImage, cmdImageState, map[string]any{"hash": hash, "confirm": false}, nil)
}

func (k *Keyboard) Reset() error {
	return k.c.Call(smp.OpWrite, smp.GroupOS, cmdOSReset, nil, nil)
}

// mcubootMagic — подпись заголовка образа MCUboot (IMAGE_MAGIC).
const mcubootMagic = 0x96f3b83d

var ErrNotImage = i18n.Error("это не образ MCUboot: нужен подписанный файл zmk.signed.bin из сборки rainy75-studio")

func ImageVersion(img []byte) (string, error) {
	if len(img) < 32 || binary.LittleEndian.Uint32(img) != mcubootMagic {
		return "", ErrNotImage
	}
	// struct image_version { u8 major; u8 minor; u16 revision; u32 build } по смещению 20.
	v := img[20:28]
	return fmt.Sprintf("%d.%d.%d+%d", v[0], v[1], binary.LittleEndian.Uint16(v[2:]), binary.LittleEndian.Uint32(v[4:])), nil
}
