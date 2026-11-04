package main

import (
	"strings"
	"testing"
	"time"
)

func testFont() *ankFont {
	var f ankFont
	for c := byte(0x20); c < 0x7f; c++ {
		for y := 0; y < 16; y++ {
			f[c][y] = c
		}
		f[c][0] = 0xFF
		f[c][15] = c
	}
	return &f
}

func paintText(font *ankFont, width, height int, lines map[int]string) []byte {
	raw := make([]byte, width*height*4)
	for row, text := range lines {
		for col := 0; col < len(text); col++ {
			ch := text[col]
			if int(ch) >= len(font) {
				continue
			}
			glyph := font[ch]
			x0, y0 := col*8, row*16
			for y := 0; y < 16; y++ {
				for x := 0; x < 8; x++ {
					if glyph[y]&(0x80>>x) == 0 {
						continue
					}
					o := ((y0+y)*width + (x0 + x)) * 4
					raw[o], raw[o+1], raw[o+2], raw[o+3] = 255, 255, 255, 255
				}
			}
		}
	}
	return raw
}

func TestPromptRow(t *testing.T) {
	if !promptRow("A>") || !promptRow("A:\\>") || !promptRow("C:\\GAME>") {
		t.Fatal("drive prompt")
	}
	if !promptRow("A:\\GAME>?") {
		t.Fatal("cursor after prompt")
	}
	if promptRow(">") || promptRow("HELLO") || promptRow("A> more") {
		t.Fatal("not a prompt")
	}
}

func TestDOSPromptOnFrame(t *testing.T) {
	font := testFont()
	const w, h = 320, 320
	raw := paintText(font, w, h, map[int]string{
		2: "MS-DOS",
		4: "A>",
	})
	if !dosPrompt(font, raw, w, h, w*4, 1) {
		t.Fatal("prompt")
	}
	game := paintText(font, w, h, map[int]string{10: "SCORE 12"})
	if dosPrompt(font, game, w, h, w*4, 1) {
		t.Fatal("game text is not a prompt")
	}
}

func TestPromptWatchReturnsAfterGame(t *testing.T) {
	font := testFont()
	const w, h = 320, 320
	boot := paintText(font, w, h, map[int]string{4: "A>"})
	game := paintText(font, w, h, map[int]string{8: "PLAYING"})
	back := paintText(font, w, h, map[int]string{18: "A:\\>"})
	clock := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	wch := &promptWatch{font: font, now: func() time.Time { return clock }}
	step := func(raw []byte, d time.Duration) bool {
		clock = clock.Add(d)
		return wch.step(raw, w, h, w*4, 1)
	}
	if step(boot, time.Second) {
		t.Fatal("boot prompt returned")
	}
	if step(boot, 11*time.Second) {
		t.Fatal("boot prompt under the stuck limit returned")
	}
	if step(boot, 2*time.Second) == false {
		t.Fatal("prompt stuck at DOS did not return")
	}
	clock = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	wch = &promptWatch{font: font, now: func() time.Time { return clock }}
	step(game, 0)
	if step(game, 7*time.Second) {
		t.Fatal("game returned")
	}
	if !wch.armed {
		t.Fatal("game did not arm")
	}
	step(back, 0)
	if step(back, time.Second) {
		t.Fatal("prompt returned too soon")
	}
	if !step(back, time.Second) {
		t.Fatal("prompt after the game did not return")
	}
}

func TestRealFontRoundTrip(t *testing.T) {
	font, err := loadANK(`\\bbs\ele\doors\pc98\bios`)
	if err != nil {
		t.Skip(err)
	}
	const w, h = 640, 400
	raw := paintText(font, w, h, map[int]string{
		1:  "MS-DOS Version 6.20",
		20: "A:\\>",
	})
	if !dosPrompt(font, raw, w, h, w*4, 1) {
		got := readRow(font, frameView{raw: raw, width: w, height: h, pitch: w * 4, pix: 1}, 20, 80)
		t.Fatalf("row %q", strings.TrimRight(got, " "))
	}
}
