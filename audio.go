package main

import "encoding/base64"

// audioAPC packs 44100 Hz stereo 16-bit PCM into a SyncTERM LoadBlob and
// queues it on mixer channel 2. prime also lifts that channel to full volume.
func audioAPC(pcm []byte, prime bool) []byte {
	pcm = pcm[:len(pcm)&^3]
	wav := make([]byte, 44+len(pcm))
	copy(wav, "RIFF")
	put32(wav[4:], uint32(36+len(pcm)))
	copy(wav[8:], "WAVEfmt ")
	put32(wav[16:], 16)
	put16(wav[20:], 1)
	put16(wav[22:], 2)
	put32(wav[24:], 44100)
	put32(wav[28:], 44100*4)
	put16(wav[32:], 4)
	put16(wav[34:], 16)
	copy(wav[36:], "data")
	put32(wav[40:], uint32(len(pcm)))
	copy(wav[44:], pcm)
	enc := base64.StdEncoding
	b64 := make([]byte, enc.EncodedLen(len(wav)))
	enc.Encode(b64, wav)
	var out []byte
	if prime {
		out = append(out, "\x1b_SyncTERM:A;Volume;C=2;V=100\x1b\\"...)
	}
	out = append(out, "\x1b_SyncTERM:A;LoadBlob;S=0;"...)
	out = append(out, b64...)
	out = append(out, "\x1b\\\x1b_SyncTERM:A;Queue;C=2;S=0;V=100\x1b\\"...)
	return out
}

// audioStop clears SyncTERM mixer channel 2, including audio still queued.
func audioStop() []byte {
	return []byte("\x1b_SyncTERM:A;Flush;C=2\x1b\\")
}

func put16(b []byte, v uint16) {
	b[0] = byte(v)
	b[1] = byte(v >> 8)
}

func put32(b []byte, v uint32) {
	b[0] = byte(v)
	b[1] = byte(v >> 8)
	b[2] = byte(v >> 16)
	b[3] = byte(v >> 24)
}
