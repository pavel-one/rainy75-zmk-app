package rainy

import (
	"bytes"
	"encoding/binary"
	"slices"
	"testing"
	"time"

	"github.com/fxamacker/cbor/v2"
)

// fakeCaller — прошивка: принимает загрузку образа и запоминает кадры.
type fakeCaller struct {
	flash []byte
	frame []byte
	calls int

	limit   int     // предел пакета, как у Bluetooth; 0 — без предела
	shown   []Color // кадр на клавиатуре в режиме компьютера; nil — эффекты
	longest int     // самый длинный пакет
}

func (f *fakeCaller) MaxPacket() int { return f.limit }

func (f *fakeCaller) Call(op byte, group uint16, id byte, req, resp any) error {
	return f.CallTimeout(time.Second, op, group, id, req, resp)
}

func (f *fakeCaller) CallTimeout(_ time.Duration, op byte, group uint16, id byte, req, resp any) error {
	f.calls++
	// Пропускаем запрос через CBOR, как по проводу.
	raw, _ := cbor.Marshal(req)
	f.longest = max(f.longest, 8+len(raw))
	var m map[string]any
	_ = cbor.Unmarshal(raw, &m)
	out := map[string]any{"rc": 0}
	switch {
	case group == 1 && id == cmdImageUpload:
		data := m["data"].([]byte)
		off := int(m["off"].(uint64))
		if off == 0 {
			f.flash = make([]byte, 0, m["len"].(uint64))
		}
		// Прошивка принимает только половину куска — клиент должен продолжить с off.
		data = data[:max(1, len(data)/2)]
		f.flash = append(f.flash[:off], data...)
		out["off"] = len(f.flash)
	case group == groupRGB && id == cmdFrame:
		f.frame = m["rgb"].([]byte)
	case group == groupRGB && id == cmdSet:
		if f.shown == nil {
			f.shown = make([]Color, Positions) // первый «set» начинает с чёрного
		}
		px := m["px"].([]byte)
		for i := 0; i < len(px); i += 4 {
			f.shown[px[i]] = Color{px[i+1], px[i+2], px[i+3]}
		}
	case group == groupRGB && id == cmdInfo:
		out["host"] = f.shown != nil
	case group == groupRGB && id == cmdClear:
		f.shown = nil
	}
	enc, _ := cbor.Marshal(out)
	if resp != nil {
		return cbor.Unmarshal(enc, resp)
	}
	return nil
}

func TestUploadFollowsDeviceOffset(t *testing.T) {
	img := bytes.Repeat([]byte("rainy75!"), 300)
	f := &fakeCaller{}
	var last int
	if err := New(f).Upload(img, func(done, total int) { last = done }); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(f.flash, img) || last != len(img) {
		t.Fatalf("записано %d из %d байт, прогресс %d", len(f.flash), len(img), last)
	}
}

func TestFrameLength(t *testing.T) {
	f := &fakeCaller{}
	kb := New(f)
	if err := kb.Frame(make([]Color, 10)); err == nil {
		t.Fatal("кадр неверной длины должен отклоняться")
	}
	frame := make([]Color, Positions)
	frame[82] = Color{1, 2, 3}
	if err := kb.Frame(frame); err != nil {
		t.Fatal(err)
	}
	if len(f.frame) != 3*Positions || !bytes.Equal(f.frame[246:], []byte{1, 2, 3}) {
		t.Fatalf("кадр: %d байт", len(f.frame))
	}
}

func TestImageVersion(t *testing.T) {
	img := make([]byte, 64)
	binary.LittleEndian.PutUint32(img, mcubootMagic)
	img[20], img[21] = 1, 2
	binary.LittleEndian.PutUint16(img[22:], 3)
	binary.LittleEndian.PutUint32(img[24:], 4)
	v, err := ImageVersion(img)
	if err != nil || v != "1.2.3+4" {
		t.Fatalf("версия %q, ошибка %v", v, err)
	}
	if _, err := ImageVersion([]byte("UF2\n not mcuboot image, just bytes")); err != ErrNotImage {
		t.Fatalf("ошибка: %v", err)
	}
}

func TestFrameOverBluetoothFitsOneWrite(t *testing.T) {
	f := &fakeCaller{limit: 62}
	kb := New(f)
	frame := make([]Color, Positions)
	for i := range frame {
		frame[i] = Color{uint8(i), 1, 2}
	}
	if err := kb.Frame(frame); err != nil {
		t.Fatal(err)
	}
	if f.longest > f.limit || f.frame != nil {
		t.Fatalf("пакет %d байт при пределе %d, команда frame: %v", f.longest, f.limit, f.frame != nil)
	}
	if !slices.Equal(f.shown, frame) {
		t.Fatal("кадр на клавиатуре не совпал")
	}

	// Следующий кадр — только изменившиеся клавиши.
	f.calls = 0
	frame[5], frame[70] = Color{9, 9, 9}, Color{8, 8, 8}
	if err := kb.Frame(frame); err != nil {
		t.Fatal(err)
	}
	if f.calls != 1 || !slices.Equal(f.shown, frame) {
		t.Fatalf("вызовов %d, кадр совпал: %v", f.calls, slices.Equal(f.shown, frame))
	}

	// Прошивка вышла из режима кадров (Fn-клавиша) — шлём кадр целиком.
	f.shown = nil
	if _, err := kb.Info(); err != nil {
		t.Fatal(err)
	}
	if err := kb.Frame(frame); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(f.shown, frame) {
		t.Fatal("после выхода из режима кадров кадр не восстановлен")
	}
}
