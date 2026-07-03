// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"fmt"
	"unicode/utf8"
)

// cborReader decodes the small CBOR subset the device's airgap files use:
// definite-length unsigned integers, arrays, and text strings (the device's
// minicbor conventions). Hand-rolled like the cborWriter - no dependency.
// Indefinite lengths and the unused major types are rejected outright.
type cborReader struct {
	buf []byte
	pos int
}

func (r *cborReader) head() (byte, uint64, error) {
	if r.pos >= len(r.buf) {
		return 0, 0, fmt.Errorf("truncated CBOR")
	}
	ib := r.buf[r.pos]
	r.pos++
	major, ai := ib>>5, ib&0x1f
	switch {
	case ai < 24:
		return major, uint64(ai), nil
	case ai <= 27:
		n := 1 << (ai - 24)
		if r.pos+n > len(r.buf) {
			return 0, 0, fmt.Errorf("truncated CBOR head")
		}
		var v uint64
		for i := 0; i < n; i++ {
			v = v<<8 | uint64(r.buf[r.pos+i])
		}
		r.pos += n
		return major, v, nil
	default:
		return 0, 0, fmt.Errorf("unsupported CBOR additional info %d", ai)
	}
}

func (r *cborReader) uint() (uint64, error) {
	major, v, err := r.head()
	if err != nil {
		return 0, err
	}
	if major != 0 {
		return 0, fmt.Errorf("expected unsigned integer, got major %d", major)
	}
	return v, nil
}

func (r *cborReader) arrayHead() (int, error) {
	major, v, err := r.head()
	if err != nil {
		return 0, err
	}
	if major != 4 {
		return 0, fmt.Errorf("expected array, got major %d", major)
	}
	// No airgap file carries arrays anywhere near this size; the cap keeps a
	// hostile length from driving allocations.
	if v > 1<<20 {
		return 0, fmt.Errorf("array too large")
	}
	return int(v), nil
}

func (r *cborReader) text(max int) (string, error) {
	major, v, err := r.head()
	if err != nil {
		return "", err
	}
	if major != 3 {
		return "", fmt.Errorf("expected text string, got major %d", major)
	}
	if v > uint64(max) {
		return "", fmt.Errorf("text too long (%d bytes)", v)
	}
	n := int(v)
	if r.pos+n > len(r.buf) {
		return "", fmt.Errorf("truncated text string")
	}
	s := string(r.buf[r.pos : r.pos+n])
	r.pos += n
	if !utf8.ValidString(s) {
		return "", fmt.Errorf("invalid UTF-8 in text string")
	}
	return s, nil
}

// skip advances over one data item of any supported shape, so decoders can
// ignore fields appended by future format versions.
func (r *cborReader) skip() error {
	major, v, err := r.head()
	if err != nil {
		return err
	}
	switch major {
	case 0, 1, 7: // integers, simple values: fully consumed by head()
		return nil
	case 2, 3: // byte / text string: payload follows
		n := int(v)
		if r.pos+n > len(r.buf) {
			return fmt.Errorf("truncated string in skip")
		}
		r.pos += n
		return nil
	case 4: // array: skip each element
		for i := uint64(0); i < v; i++ {
			if err := r.skip(); err != nil {
				return err
			}
		}
		return nil
	case 5: // map: skip each key/value pair
		for i := uint64(0); i < 2*v; i++ {
			if err := r.skip(); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("cannot skip CBOR major %d", major)
	}
}
