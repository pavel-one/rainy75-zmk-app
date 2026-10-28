package smp

import (
	"encoding/binary"
	"fmt"
	"io"
	"rainy75/internal/i18n"
	"sync"
	"time"

	"github.com/fxamacker/cbor/v2"
)

const (
	OpRead  = 0
	OpWrite = 2
)

const (
	GroupOS    = 0
	GroupImage = 1
)

const headerSize = 8

var (
	// ErrTimeout — клавиатура не ответила вовремя.
	ErrTimeout = i18n.Error("клавиатура не ответила")
	// ErrIO — сбой обмена с портом (например, кабель отключили).
	ErrIO = i18n.Error("ошибка обмена с клавиатурой")
)

var mgmtErrors = map[int]string{
	1:  "неизвестная ошибка",
	2:  "не хватает памяти",
	3:  "неверное значение",
	4:  "истекло время ожидания",
	5:  "не найдено",
	6:  "неподходящее состояние",
	7:  "слишком большое сообщение",
	8:  "команда не поддерживается прошивкой",
	9:  "данные повреждены",
	10: "устройство занято",
}

type MgmtError struct {
	Group uint16
	Code  int
}

func (e *MgmtError) Error() string {
	msg, ok := mgmtErrors[e.Code]
	if !ok {
		msg = "ошибка"
	}
	return i18n.Sprintf("клавиатура: %s (группа %d, код %d)", i18n.T(msg), e.Group, e.Code)
}

// Port — последовательный порт с таймаутом чтения (go.bug.st/serial).
type Port interface {
	io.ReadWriteCloser
	SetReadTimeout(t time.Duration) error
}

type framing interface {
	encode(packet []byte) []byte
	feed(p []byte) ([][]byte, error)
}

// serialFraming — строки base64 с маркерами (последовательный порт).
type serialFraming struct{ decoder }

func (serialFraming) encode(packet []byte) []byte { return encodeFrame(packet) }

// rawFraming — пакеты как есть (Bluetooth): границы — по длине из заголовка.
type rawFraming struct{ buf []byte }

func (*rawFraming) encode(packet []byte) []byte { return packet }

func (r *rawFraming) feed(p []byte) ([][]byte, error) {
	r.buf = append(r.buf, p...)
	var out [][]byte
	for len(r.buf) >= headerSize {
		total := headerSize + int(binary.BigEndian.Uint16(r.buf[2:]))
		if len(r.buf) < total {
			break
		}
		out = append(out, append([]byte(nil), r.buf[:total]...))
		r.buf = r.buf[total:]
	}
	return out, nil
}

// Client отправляет SMP-запросы. Безопасен для конкурентного использования.
type Client struct {
	mu      sync.Mutex
	p       Port
	seq     uint8
	dec     framing
	Timeout time.Duration
}

func NewClient(p Port) *Client {
	return &Client{p: p, dec: &serialFraming{}, Timeout: 3 * time.Second}
}

// NewRawClient создаёт клиента поверх транспорта без обёртки (Bluetooth).
func NewRawClient(p Port) *Client {
	return &Client{p: p, dec: &rawFraming{}, Timeout: 3 * time.Second}
}

func (c *Client) Close() error { return c.p.Close() }

// Call отправляет запрос (req — CBOR-карта или nil) и раскладывает ответ в resp.
func (c *Client) Call(op byte, group uint16, id byte, req, resp any) error {
	return c.CallTimeout(c.Timeout, op, group, id, req, resp)
}

// CallTimeout — Call с собственным временем ожидания (стирание flash при
// загрузке прошивки занимает секунды).
func (c *Client) CallTimeout(timeout time.Duration, op byte, group uint16, id byte, req, resp any) error {
	if req == nil {
		req = map[string]any{}
	}
	body, err := cbor.Marshal(req)
	if err != nil {
		return i18n.Errorf("SMP: кодирование запроса: %w", err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.seq++
	hdr := make([]byte, headerSize, headerSize+len(body))
	hdr[0] = op
	binary.BigEndian.PutUint16(hdr[2:], uint16(len(body)))
	binary.BigEndian.PutUint16(hdr[4:], group)
	hdr[6], hdr[7] = c.seq, id
	if _, err := c.p.Write(c.dec.encode(append(hdr, body...))); err != nil {
		return i18n.Errorf("%w: запись: %v", ErrIO, err)
	}

	payload, err := c.await(timeout, op+1, group, id, c.seq)
	if err != nil {
		return err
	}
	var status struct {
		RC  int `cbor:"rc"`
		Err *struct {
			Group uint16 `cbor:"group"`
			RC    int    `cbor:"rc"`
		} `cbor:"err"` // формат ошибок SMP v2
	}
	if err := cbor.Unmarshal(payload, &status); err != nil {
		return i18n.Errorf("SMP: разбор ответа: %w", err)
	}
	if status.RC != 0 {
		return &MgmtError{Group: group, Code: status.RC}
	}
	if status.Err != nil && status.Err.RC != 0 {
		return &MgmtError{Group: status.Err.Group, Code: status.Err.RC}
	}
	if resp != nil {
		if err := cbor.Unmarshal(payload, resp); err != nil {
			return i18n.Errorf("SMP: разбор ответа: %w", err)
		}
	}
	return nil
}

// await читает порт, пока не придёт ответ на запрос seq.
func (c *Client) await(timeout time.Duration, op byte, group uint16, id, seq byte) ([]byte, error) {
	stop := time.Now().Add(timeout)
	buf := make([]byte, 512)
	for time.Now().Before(stop) {
		if err := c.p.SetReadTimeout(min(100*time.Millisecond, time.Until(stop))); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrIO, err)
		}
		n, err := c.p.Read(buf)
		if err != nil {
			return nil, i18n.Errorf("%w: чтение: %v", ErrIO, err)
		}
		packets, _ := c.dec.feed(buf[:n]) // битый кадр — ждём следующий
		for _, pkt := range packets {
			if len(pkt) < headerSize {
				continue
			}
			// Запоздавшие ответы на прошлые запросы отбрасываем.
			if pkt[0] != op || binary.BigEndian.Uint16(pkt[4:]) != group || pkt[6] != seq || pkt[7] != id {
				continue
			}
			size := int(binary.BigEndian.Uint16(pkt[2:]))
			if len(pkt) < headerSize+size {
				return nil, i18n.Error("SMP: ответ короче заявленного")
			}
			return pkt[headerSize : headerSize+size], nil
		}
	}
	return nil, ErrTimeout
}
