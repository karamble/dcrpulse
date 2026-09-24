// Copyright (c) 2013-2016 Company 0, LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.
//
// Copied verbatim from github.com/companyzero/bisonrelay/internal/audio at
// commit 713f0ec5cfe4ff94fb5cd3a490cd09ea3fa1f91d; only the package name and
// the serial seam in newOggWriter differ.

package oggopus

import (
	"encoding/binary"
	"fmt"
	"io"
)

const (
	opusIdSig      = "OpusHead"
	opusCommentSig = "OpusTags"
)

type OpusPacket []byte

type opusWriter struct {
	ogg *oggWriter

	totalPCMSamples uint64
	pageIndex       uint32
}

func newOpusWriter(out io.Writer) (*opusWriter, error) {
	return newOpusWriterOgg(newOggWriter(out))
}

func newOpusWriterSerial(out io.Writer, serial uint32) (*opusWriter, error) {
	return newOpusWriterOgg(newOggWriterSerial(out, serial))
}

func newOpusWriterOgg(oggWriter *oggWriter) (*opusWriter, error) {

	writer := &opusWriter{
		ogg: oggWriter,
	}

	err := writer.writeHeaders()
	if err != nil {
		return nil, err
	}

	return writer, nil
}

func (w *opusWriter) writeHeaders() error {
	idHeader := make([]byte, 19)
	copy(idHeader[0:], opusIdSig)
	idHeader[8] = 1
	idHeader[9] = 2

	binary.LittleEndian.PutUint16(idHeader[10:], 0 /*312*/) // pre-skip, this is what ffmpeg / libopus seems to like
	binary.LittleEndian.PutUint32(idHeader[12:], 48000)     // sample rate
	binary.LittleEndian.PutUint16(idHeader[16:], 0)         // output gain
	idHeader[18] = 0                                        // mono or stereo

	idPage := w.ogg.NewPage(idHeader, 0, w.pageIndex)
	idPage.IsFirstPage = true
	err := w.ogg.WritePage(idPage)
	if err != nil {
		return err
	}
	w.pageIndex++

	commentHeader := make([]byte, 25)
	copy(commentHeader[0:], opusCommentSig)
	binary.LittleEndian.PutUint32(commentHeader[8:], 9)  // vendor name length
	copy(commentHeader[12:], "skynetbot")                // vendor name
	binary.LittleEndian.PutUint32(commentHeader[21:], 0) // comment list Length

	commentPage := w.ogg.NewPage(commentHeader, 0, w.pageIndex)
	err = w.ogg.WritePage(commentPage)
	if err == nil {
		w.pageIndex++
	}
	return err
}

func (w *opusWriter) WritePacket(p []byte, pcmSamples uint64, isLast bool) error {
	if len(p) > 255*255 {
		// Such a large payload requires splitting a single packet into
		// multiple ogg pages.
		return fmt.Errorf("packet splitting not supported")
	}
	granule := w.totalPCMSamples + pcmSamples
	w.totalPCMSamples += pcmSamples
	page := w.ogg.NewPage(p, granule, w.pageIndex)
	page.IsLastPage = isLast
	w.pageIndex++

	return w.ogg.WritePage(page)
}
