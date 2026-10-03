package main

import "unsafe"

func uintptrPointer[T any](p *T) uintptr { return uintptr(unsafe.Pointer(p)) }

// A 16x16 32-bit ICO, generated in memory; no external asset is needed at runtime.
func icon() []byte {
	b := make([]byte, 22+40+16*16*4+16*4)
	b[2] = 1
	b[4] = 1
	b[6] = 16
	b[7] = 16
	b[10] = 1
	b[12] = 32
	size := len(b) - 22
	for i := 0; i < 4; i++ {
		b[14+i] = byte(size >> (8 * i))
	}
	b[18] = 22
	b[22] = 40
	b[26] = 16
	b[30] = 32
	b[34] = 1
	b[36] = 32
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			i := 62 + (y*16+x)*4
			b[i] = 48
			b[i+1] = 102
			b[i+2] = 28
			b[i+3] = 255
			if x >= 4 && x <= 11 && y >= 4 && y <= 11 {
				b[i] = 210
				b[i+1] = 240
				b[i+2] = 240
			}
		}
	}
	return b
}
