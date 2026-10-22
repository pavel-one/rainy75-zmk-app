package smp

import (
	"bytes"
	"encoding/binary"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/fxamacker/cbor/v2"
)

// fakePort — прошивка на другом конце порта: разбирает запросы и отвечает
// функцией handle. Перед ответом пишет строку консоли, как настоящая.
type fakePort struct {
	mu     sync.Mutex
	in     decoder
	out    bytes.Buffer
	handle func(hdr []byte, req map[string]any) (map[string]any, bool)
	stale  bool // сначала прислать ответ на «прошлый» запрос
}

func (p *fakePort) Write(b []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	pkts, err := p.in.feed(b)
	if err != nil {
		return 0, err
	}
	for _, pkt := range pkts {
		hdr := pkt[:headerSize]
		var req map[string]any
		if err := cbor.Unmarshal(pkt[headerSize:], &req); err != nil {
			return 0, err
		}
		resp, ok := p.handle(hdr, req)
		if !ok {
			continue
		}
		p.out.WriteString("[00:00:01.000] <inf> rgb_mgmt: лог консоли\r\n")
		if p.stale {
			old := append([]byte(nil), hdr...)
			old[6]--
			p.reply(old, map[string]any{"rc": 0, "stale": true})
		}
		p.reply(hdr, resp)
	}
	return len(b), nil
}

func (p *fakePort) reply(hdr []byte, resp map[string]any) {
	body, _ := cbor.Marshal(resp)
	h := append([]byte(nil), hdr...)
	h[0]++
	binary.BigEndian.PutUint16(h[2:], uint16(len(body)))
	p.out.Write(encodeFrame(append(h, body...)))
}

func (p *fakePort) Read(b []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.out.Len() == 0 {
		time.Sleep(time.Millisecond)
		return 0, nil
	}
	return p.out.Read(b)
}

func (p *fakePort) SetReadTimeout(time.Duration) error { return nil }
func (p *fakePort) Close() error                       { return nil }

func TestCRC16XModem(t *testing.T) {
	if got := crc16([]byte("123456789")); got != 0x31C3 {
		t.Fatalf("crc16 = %#04x, ожидалось 0x31C3", got)
	}
}

func TestFrameRoundTripLong(t *testing.T) {
	pkt := bytes.Repeat([]byte{0xAB, 0x01, 0x7F}, 200) // несколько строк продолжения
	lines := encodeFrame(pkt)
	for _, l := range bytes.Split(bytes.TrimSuffix(lines, []byte("\n")), []byte("\n")) {
		if len(l)+1 > 127 {
			t.Fatalf("строка длиннее 127 байт: %d", len(l)+1)
		}
	}
	var d decoder
	// Подаём по одному байту, вперемешку с выводом консоли.
	var got [][]byte
	for _, chunk := range [][]byte{[]byte("boot ok\n"), lines} {
		for _, b := range chunk {
			pkts, err := d.feed([]byte{b})
			if err != nil {
				t.Fatal(err)
			}
			got = append(got, pkts...)
		}
	}
	if len(got) != 1 || !bytes.Equal(got[0], pkt) {
		t.Fatalf("получено %d пакетов", len(got))
	}
}

func TestCallMatchesSeqAndDecodes(t *testing.T) {
	port := &fakePort{stale: true, handle: func(hdr []byte, req map[string]any) (map[string]any, bool) {
		if binary.BigEndian.Uint16(hdr[4:]) != 65 || hdr[7] != 3 || hdr[0] != OpRead {
			return map[string]any{"rc": 8}, true
		}
		return map[string]any{"rc": 0, "n": 83, "host": false, "win": true}, true
	}}
	c := NewClient(port)
	var info struct {
		N     int  `cbor:"n"`
		Win   bool `cbor:"win"`
		Stale bool `cbor:"stale"`
	}
	if err := c.Call(OpRead, 65, 3, nil, &info); err != nil {
		t.Fatal(err)
	}
	if info.N != 83 || !info.Win || info.Stale {
		t.Fatalf("ответ: %+v", info)
	}
}

func TestCallReturnsMgmtError(t *testing.T) {
	port := &fakePort{handle: func([]byte, map[string]any) (map[string]any, bool) {
		return map[string]any{"rc": 3}, true
	}}
	err := NewClient(port).Call(OpWrite, 65, 1, map[string]any{"r": 300}, nil)
	var me *MgmtError
	if !errors.As(err, &me) || me.Code != 3 {
		t.Fatalf("ошибка: %v", err)
	}
}

func TestRawFramingSplitsPackets(t *testing.T) {
	var r rawFraming
	a := append([]byte{3, 0, 0, 2, 0, 65, 1, 3}, 0xA0, 0xA1)
	b := []byte{3, 0, 0, 0, 0, 65, 2, 3}
	stream := append(append([]byte(nil), a...), b...)
	var got [][]byte
	for _, c := range stream { // по байту, как фрагменты уведомлений
		pk, _ := r.feed([]byte{c})
		got = append(got, pk...)
	}
	if len(got) != 2 || !bytes.Equal(got[0], a) || !bytes.Equal(got[1], b) {
		t.Fatalf("пакеты: % x", got)
	}
}

func TestCallTimeout(t *testing.T) {
	port := &fakePort{handle: func([]byte, map[string]any) (map[string]any, bool) { return nil, false }}
	c := NewClient(port)
	c.Timeout = 50 * time.Millisecond
	if err := c.Call(OpRead, 65, 3, nil, nil); !errors.Is(err, ErrTimeout) {
		t.Fatalf("ошибка: %v", err)
	}
}
