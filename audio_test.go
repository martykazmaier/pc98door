package main

import (
	"bytes"
	"encoding/base64"
	"testing"
)

func TestAudioAPC(t *testing.T) {
	pcm := []byte{1, 0, 2, 0, 3, 0, 4, 0}
	blob := audioAPC(pcm, true)
	if !bytes.Contains(blob, []byte("SyncTERM:A;Volume;C=2;V=100")) {
		t.Fatal("missing volume")
	}
	if !bytes.Contains(blob, []byte("SyncTERM:A;LoadBlob;S=0;")) {
		t.Fatal("missing load")
	}
	if !bytes.Contains(blob, []byte("SyncTERM:A;Queue;C=2;S=0;V=100")) {
		t.Fatal("missing queue")
	}
	i := bytes.Index(blob, []byte("LoadBlob;S=0;"))
	raw := blob[i+len("LoadBlob;S=0;"):]
	j := bytes.IndexByte(raw, 0x1b)
	dec := make([]byte, base64.StdEncoding.DecodedLen(j))
	n, err := base64.StdEncoding.Decode(dec, raw[:j])
	if err != nil {
		t.Fatal(err)
	}
	dec = dec[:n]
	if string(dec[:4]) != "RIFF" || string(dec[8:16]) != "WAVEfmt " {
		t.Fatalf("wav header %q", dec[:16])
	}
	if int(dec[24])|int(dec[25])<<8|int(dec[26])<<16|int(dec[27])<<24 != 44100 {
		t.Fatal("rate")
	}
	if !bytes.Equal(dec[44:], pcm) {
		t.Fatal("pcm")
	}
	if !bytes.Contains(audioStop(), []byte("SyncTERM:A;Flush;C=2")) {
		t.Fatal("stop")
	}
}
