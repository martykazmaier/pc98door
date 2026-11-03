package main

import (
	"fmt"
	"io"
	"strings"
	"time"
)

const (
	keyNone  = 0
	keyBack  = 8
	keyEnter = 13
	keyEsc   = 27
	keyCtrlQ = 17
	keyUp    = 0x101
	keyDown  = 0x102
	keyLeft  = 0x103
	keyRight = 0x104
	keyHome  = 0x105
	keyEnd   = 0x106
	keyPgUp  = 0x107
	keyPgDn  = 0x108
	keyDel   = 0x10A
	keyShift = 0x10000
	keyCtrl  = 0x20000

	menuCols = 79
	// 7 frame lines plus the title list. 24 lines fit a 25-line terminal;
	// a 25th line scrolls the top border off the screen.
	menuRows = 17
)

type keyboard interface {
	WaitKey(time.Duration) (int, bool, error)
}

type picker struct {
	user  string
	all   []diskTitle
	query string
	sel   int
	top   int
}

func (p *picker) visible() []diskTitle { return filterDisks(p.all, p.query) }

func (p *picker) clamp(n int) {
	if n <= 0 {
		p.sel, p.top = 0, 0
		return
	}
	if p.sel < 0 {
		p.sel = 0
	}
	if p.sel >= n {
		p.sel = n - 1
	}
	if p.sel < p.top {
		p.top = p.sel
	}
	if p.sel >= p.top+menuRows {
		p.top = p.sel - menuRows + 1
	}
}

func (p *picker) pageDown(n int) {
	if n <= 0 {
		return
	}
	bottom := p.top + menuRows - 1
	if bottom >= n {
		bottom = n - 1
	}
	if p.sel < bottom {
		p.sel = bottom
		return
	}
	maxTop := n - menuRows
	if maxTop < 0 {
		maxTop = 0
	}
	p.top += menuRows
	if p.top > maxTop {
		p.top = maxTop
	}
	p.sel = p.top + menuRows - 1
	if p.sel >= n {
		p.sel = n - 1
	}
}

func (p *picker) pageUp(n int) {
	if n <= 0 {
		return
	}
	if p.sel > p.top {
		p.sel = p.top
		return
	}
	p.top -= menuRows
	if p.top < 0 {
		p.top = 0
	}
	p.sel = p.top
}

func (p *picker) handle(k int) (chosen string, quit bool) {
	k &^= keyShift | keyCtrl
	n := len(p.visible())
	switch k {
	case keyCtrlQ:
		return "", true
	case keyEsc:
		if p.query != "" {
			p.query = ""
			p.sel, p.top = 0, 0
			return "", false
		}
		return "", true
	case keyBack, keyDel:
		if p.query != "" {
			p.query = p.query[:len(p.query)-1]
			p.sel, p.top = 0, 0
		}
	case keyUp:
		p.sel--
	case keyDown:
		p.sel++
	case keyPgUp:
		p.pageUp(n)
	case keyPgDn:
		p.pageDown(n)
	case keyHome:
		p.sel, p.top = 0, 0
	case keyEnd:
		p.sel = n - 1
		p.top = n - menuRows
		if p.top < 0 {
			p.top = 0
		}
	case keyEnter:
		vis := p.visible()
		p.clamp(len(vis))
		if len(vis) == 0 {
			return "", false
		}
		return vis[p.sel].Name, false
	default:
		if k >= 32 && k <= 255 {
			p.query += string(byte(k))
			p.sel, p.top = 0, 0
		}
	}
	p.clamp(len(p.visible()))
	return "", false
}

func (p *picker) frame() []byte {
	vis := p.visible()
	p.clamp(len(vis))
	var b strings.Builder
	b.WriteString("\x1b[0m\x1b[?7l\x1b[2J\x1b[H")
	write := func(s string) {
		b.WriteString(s)
		b.WriteString("\r\n")
	}
	write(boxLine(0xC9, 0xCD, 0xBB))
	inner := menuCols - 2
	user := fitBytes(utf8ToCP437(p.user), 40)
	left := " pc98door"
	gap := inner - len(left) - len(user)
	if gap < 1 {
		gap = 1
		user = fitBytes(user, inner-len(left)-1)
	}
	write(textLine("\x1b[1;37m", left+strings.Repeat(" ", gap)+user))
	q := p.query
	if room := inner - len(" Search: "); len(q) > room {
		q = q[len(q)-room:]
	}
	write(textLine("\x1b[33m", fitBytes(" Search: "+q, inner)))
	write(boxLine(0xCC, 0xCD, 0xB9))
	if len(vis) == 0 {
		msg := " The roms directory is empty"
		if p.query != "" {
			msg = " No titles match"
		}
		write(textLine("\x1b[37m", fitBytes(msg, inner)))
		for i := 1; i < menuRows; i++ {
			write(textLine("\x1b[37m", strings.Repeat(" ", inner)))
		}
	} else {
		for i := 0; i < menuRows; i++ {
			n := p.top + i
			if n >= len(vis) {
				write(textLine("\x1b[37m", strings.Repeat(" ", inner)))
				continue
			}
			color := "\x1b[37;40m"
			if n == p.sel {
				color = "\x1b[1;37;44m"
			}
			write(textLine(color, fitBytes(" "+utf8ToCP437(vis[n].Title), inner)))
		}
	}
	write(boxLine(0xCC, 0xCD, 0xB9))
	hint := "PgUp/PgDn  Home/End  Enter  Esc  Ctrl-Q quits the game"
	if p.query != "" {
		hint = "PgUp/PgDn  Home/End  Enter  Esc clears  Ctrl-Q quits the game"
	}
	status := fmt.Sprintf(" %d of %d   %s", len(vis), len(p.all), hint)
	write(textLine("\x1b[36m", fitBytes(status, inner)))
	write(boxLine(0xC8, 0xCD, 0xBC))
	fmt.Fprintf(&b, "\x1b[%d;%dH", 3, 11+len(q))
	return []byte(b.String())
}

func boxLine(left, fill, right byte) string {
	row := make([]byte, menuCols)
	row[0] = left
	for i := 1; i < menuCols-1; i++ {
		row[i] = fill
	}
	row[menuCols-1] = right
	return "\x1b[36m" + string(row) + "\x1b[0m"
}

func textLine(color, inner string) string {
	return "\x1b[36m\xBA\x1b[0m" + color + inner + "\x1b[0m\x1b[36m\xBA\x1b[0m"
}

func chooseDisk(w io.Writer, keys keyboard, user string, disks []diskTitle) (string, error) {
	p := &picker{user: user, all: disks}
	if _, err := w.Write(p.frame()); err != nil {
		return "", err
	}
	for {
		k, ok, err := keys.WaitKey(-1)
		if err != nil {
			return "", err
		}
		if !ok {
			continue
		}
		name, quit := p.handle(k)
		if quit {
			return "", nil
		}
		if name != "" {
			return name, nil
		}
		if _, err := w.Write(p.frame()); err != nil {
			return "", err
		}
	}
}
