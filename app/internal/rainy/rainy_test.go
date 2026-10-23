package rainy

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"

	"github.com/fxamacker/cbor/v2"
)

// fakeCaller — прошивка: принимает загрузку образа и запоминает кадры.
type fakeCaller struct {
	flash []byte
	frame []byte
	calls int
}

func (f *fakeCaller) Call(op byte, group uint16, id byte, req, resp any) error {
	return f.CallTimeout(time.Second, op, group, id, req, resp)
}

func (f *fakeCaller) CallTimeout(_ time.Duration, op byte, group uint16, id byte, req, resp any) error {
	f.calls++
	// Пропускаем запрос через CBOR, как по проводу.
	raw, _ := cbor.Marshal(req)
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
