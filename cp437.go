package main

import "strings"

const cp437High = "ÇüéâäàåçêëèïîìÄÅÉæÆôöòûùÿÖÜ¢£¥₧ƒáíóúñÑªº¿⌐¬½¼¡«»░▒▓│┤╡╢╖╕╣║╗╝╜╛┐└┴┬├─┼╞╟╚╔╩╦╠═╬╧╨╤╥╙╘╒╓╫╪┘┌█▄▌▐▀αßΓπΣσµτΦΘΩδ∞φε∩≡±≥≤⌠⌡÷≈°∙·√ⁿ²■\u00a0"

var (
	runeToCP437 = map[rune]byte{}
	cp437ToRune [256]rune
)

func init() {
	for i := 0; i < 128; i++ {
		cp437ToRune[i] = rune(i)
	}
	i := 0x80
	for _, r := range cp437High {
		runeToCP437[r] = byte(i)
		cp437ToRune[i] = r
		i++
	}
	if i != 0x100 {
		panic("cp437 table must have 128 entries")
	}
}

func utf8ToCP437(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r < 0x80 {
			b.WriteByte(byte(r))
		} else if c, ok := runeToCP437[r]; ok {
			b.WriteByte(c)
		} else {
			b.WriteByte('?')
		}
	}
	return b.String()
}

func asciiFold(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}

func fitBytes(s string, n int) string {
	if n <= 0 {
		return ""
	}
	if len(s) > n {
		if n <= 3 {
			return s[:n]
		}
		return s[:n-3] + "..."
	}
	return s + strings.Repeat(" ", n-len(s))
}
