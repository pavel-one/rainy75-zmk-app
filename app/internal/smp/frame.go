// Package smp — клиент протокола SMP (mcumgr) поверх последовательного порта.
// Так с хостом общается открытая прошивка Rainy75 (rainy75-studio): тот же
// канал используется для обновления через MCUboot и для группы 65 (подсветка
// и режим Win/Mac).
package smp

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"rainy75/internal/i18n"
)

var (
	markerStart = []byte{0x06, 0x09}
	markerCont  = []byte{0x04, 0x14}
)

// maxLine — длина строки транспорта без маркера и перевода строки
// (строка целиком не длиннее 127 байт), кратна 4 для base64.
const maxLine = 124

// crc16 — CRC-16/XMODEM (полином 0x1021, начальное значение 0).
func crc16(data []byte) uint16 {
	var crc uint16
	for _, b := range data {
		crc ^= uint16(b) << 8
		for range 8 {
			if crc&0x8000 != 0 {
				crc = crc<<1 ^ 0x1021
			} else {
				crc <<= 1
			}
		}
	}
	return crc
}

// encodeFrame упаковывает SMP-пакет в строки транспорта:
// base64(длина u16 BE + пакет + CRC16 BE), порезанный на строки с маркерами.
func encodeFrame(packet []byte) []byte {
	raw := make([]byte, 0, len(packet)+4)
	raw = binary.BigEndian.AppendUint16(raw, uint16(len(packet)+2))
	raw = append(raw, packet...)
	raw = binary.BigEndian.AppendUint16(raw, crc16(packet))
	b64 := base64.StdEncoding.EncodeToString(raw)

	var out bytes.Buffer
	for i := 0; i < len(b64); i += maxLine {
		if i == 0 {
			out.Write(markerStart)
		} else {
			out.Write(markerCont)
		}
		out.WriteString(b64[i:min(i+maxLine, len(b64))])
		out.WriteByte('\n')
	}
	return out.Bytes()
}

var errBadCRC = i18n.Error("SMP: неверная контрольная сумма")

// decoder собирает SMP-пакеты из потока байт. Строки без маркеров —
// это вывод консоли прошивки (она делит порт с mcumgr), их пропускаем.
type decoder struct {
	line []byte
	b64  []byte
	open bool
}

// feed добавляет байты из порта и возвращает собранные пакеты. Битый кадр
// пропускается, разбор продолжается; ошибка — последняя из встреченных.
func (d *decoder) feed(p []byte) (packets [][]byte, lastErr error) {
	for _, c := range p {
		if c != '\n' {
			d.line = append(d.line, c)
			continue
		}
		line := bytes.TrimRight(d.line, "\r")
		d.line = d.line[:0]
		switch {
		case bytes.HasPrefix(line, markerStart):
			d.b64 = append(d.b64[:0], line[2:]...)
			d.open = true
		case bytes.HasPrefix(line, markerCont) && d.open:
			d.b64 = append(d.b64, line[2:]...)
		default:
			continue
		}
		pkt, done, err := d.tryDecode()
		if err != nil {
			d.open = false
			lastErr = err
			continue
		}
		if done {
			d.open = false
			packets = append(packets, pkt)
		}
	}
	return packets, lastErr
}

func (d *decoder) tryDecode() ([]byte, bool, error) {
	if len(d.b64)%4 != 0 {
		return nil, false, nil
	}
	raw, err := base64.StdEncoding.DecodeString(string(d.b64))
	if err != nil || len(raw) < 2 {
		return nil, false, nil
	}
	n := int(binary.BigEndian.Uint16(raw))
	if len(raw) < 2+n {
		return nil, false, nil // ждём продолжения
	}
	if n < 2 {
		return nil, false, errBadCRC
	}
	pkt := raw[2:n]
	if crc16(pkt) != binary.BigEndian.Uint16(raw[n:]) {
		return nil, false, errBadCRC
	}
	return pkt, true, nil
}
