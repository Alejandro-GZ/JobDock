package agent

import "bytes"

type streamRedactor struct {
	secrets [][]byte
	pending []byte
	max     int
}

func newRedactor(values map[string]string) *streamRedactor {
	result := &streamRedactor{}
	for _, value := range values {
		if value == "" {
			continue
		}
		raw := []byte(value)
		result.secrets = append(result.secrets, raw)
		if len(raw) > result.max {
			result.max = len(raw)
		}
	}
	return result
}
func (r *streamRedactor) Push(data []byte) (safe []byte) {
	if r.max == 0 {
		return append([]byte(nil), data...)
	}
	combined := append(append([]byte(nil), r.pending...), data...)
	keep := r.max - 1
	if len(combined) <= keep {
		r.pending = combined
		return nil
	}
	cut := len(combined) - keep
	masked := r.mask(combined)
	safe = append([]byte(nil), masked[:cut]...)
	r.pending = append([]byte(nil), masked[cut:]...)
	return safe
}
func (r *streamRedactor) Flush() []byte { masked := r.mask(r.pending); r.pending = nil; return masked }
func (r *streamRedactor) mask(data []byte) []byte {
	result := append([]byte(nil), data...)
	for _, secret := range r.secrets {
		start := 0
		for {
			index := bytes.Index(result[start:], secret)
			if index < 0 {
				break
			}
			index += start
			for offset := range secret {
				result[index+offset] = '*'
			}
			start = index + len(secret)
		}
	}
	return result
}
