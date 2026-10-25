package host

import (
	"math"
	"math/cmplx"
)

// Spectrum превращает отсчёты звука в уровни полос (0…1) для визуализатора:
// БПФ, логарифмическая шкала частот, плавный спад и автоусиление.
type Spectrum struct {
	bands []float64
	peak  float64 // скользящий максимум для автоусиления
}

func NewSpectrum(n int) *Spectrum { return &Spectrum{bands: make([]float64, n), peak: 1e-3} }

// fftSize — длина окна БПФ (степень двойки): ~21 мс при 48 кГц.
const fftSize = 1024

func (s *Spectrum) Update(samples []float32, rate float64) []float64 {
	if len(samples) < fftSize || rate <= 0 {
		return s.bands
	}
	samples = samples[len(samples)-fftSize:]
	buf := make([]complex128, fftSize)
	for i, v := range samples {
		hann := 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/fftSize) // окно Ханна убирает «звон»
		buf[i] = complex(float64(v)*hann, 0)
	}
	fft(buf)

	// Полосы от 40 Гц до 16 кГц через равные доли октавы.
	n := len(s.bands)
	lo, hi := 40.0, math.Min(16000, rate/2)
	raw := make([]float64, n)
	maxv := 0.0
	for b := range n {
		f0 := lo * math.Pow(hi/lo, float64(b)/float64(n))
		f1 := lo * math.Pow(hi/lo, float64(b+1)/float64(n))
		i0 := max(1, int(f0/rate*fftSize))
		i1 := max(i0+1, int(f1/rate*fftSize))
		sum := 0.0
		for i := i0; i < i1 && i < fftSize/2; i++ {
			sum += cmplx.Abs(buf[i])
		}
		raw[b] = math.Log1p(sum / float64(i1-i0) * 8) // сжатие динамики
		maxv = math.Max(maxv, raw[b])
	}
	// Автоусиление: быстро догоняет громкий звук, медленно отпускает.
	if maxv > s.peak {
		s.peak = maxv
	} else {
		s.peak = math.Max(1e-3, s.peak*0.995)
	}
	for b := range n {
		v := raw[b] / s.peak
		if v > s.bands[b] {
			s.bands[b] = v
		} else {
			s.bands[b] = s.bands[b]*0.82 + v*0.18 // плавный спад
		}
	}
	out := make([]float64, n)
	copy(out, s.bands)
	return out
}

// fft — быстрое преобразование Фурье на месте (радикс-2, длина — степень двойки).
func fft(a []complex128) {
	n := len(a)
	for i, j := 1, 0; i < n; i++ {
		bit := n >> 1
		for ; j&bit != 0; bit >>= 1 {
			j ^= bit
		}
		j ^= bit
		if i < j {
			a[i], a[j] = a[j], a[i]
		}
	}
	for size := 2; size <= n; size <<= 1 {
		w := cmplx.Exp(complex(0, -2*math.Pi/float64(size)))
		for start := 0; start < n; start += size {
			wk := complex(1, 0)
			for k := range size / 2 {
				u, v := a[start+k], a[start+k+size/2]*wk
				a[start+k], a[start+k+size/2] = u+v, u-v
				wk *= w
			}
		}
	}
}
