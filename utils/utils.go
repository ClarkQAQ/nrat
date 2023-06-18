package utils

import (
	"bytes"
	"errors"
	"uw/uboot"
)

// 断言 Context
func UbootGetAssert[T any](c *uboot.Context, key string) (T, bool) {
	v, ok := c.Load(key)
	if !ok || v == nil {
		var empty T
		return empty, false
	}

	ret, ok := v.(T)
	return ret, ok
}

func CutMore(s string, n int) string {
	if len(s) < 2*n {
		return s
	}

	return s[:n] + "..." + s[len(s)-n:]
}

func ReadEmbedData(b []byte, none byte, startMagic, endMagic []byte) ([]byte, error) {
	startIndex, endIndex := bytes.Index(b, startMagic), bytes.Index(b, endMagic)
	if startIndex < 0 || startIndex+len(startMagic) > len(b) {
		return nil, errors.New("start magic not found")
	}
	if endIndex < 0 {
		return nil, errors.New("end magic not found")
	}

	b = b[startIndex+len(startMagic) : endIndex]
	b = bytes.TrimSpace(b)

	for i := 0; i < len(b); i++ {
		if b[i] != none {
			b = b[i:]
			break
		}
	}

	for i := len(b) - 1; i >= 0; i-- {
		if b[i] != none {
			b = b[:i+1]
			break
		}
	}

	return b, nil
}

func WriteEmbedData(b []byte, none byte, startMagic, endMagic, data []byte) ([]byte, error) {
	startIndex, endIndex := bytes.Index(b, startMagic), bytes.Index(b, endMagic)
	if startIndex < 0 || startIndex+len(startMagic) > len(b) {
		return nil, errors.New("start magic not found")
	}
	if endIndex < 0 {
		return nil, errors.New("end magic not found")
	}

	if len(data) > endIndex-startIndex-len(startMagic) {
		return nil, errors.New("data too long")
	}

	if len(data) < endIndex-startIndex-len(startMagic) {
		data = append(data, bytes.Repeat([]byte{none},
			endIndex-startIndex-len(startMagic)-len(data))...)
	}

	if n := copy(b[startIndex+len(startMagic):endIndex], data); n != len(data) {
		return nil, errors.New("copy data failed")
	}

	return b, nil
}
