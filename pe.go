package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode"
	"unsafe"
)

type Terminal interface {
	editorReadKey() int
	getWindowSize() (rows, cols int, err error)
}

var termOut *os.File = os.Stdout
var term Terminal = Console{}

type Console struct{}

func (c Console) getWindowSize() (rows, cols int, err error) {
	w := struct {
		Row, Col       uint16
		Xpixel, Ypixel uint16
	}{}
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL,
		termOut.Fd(),
		syscall.TIOCGWINSZ,
		uintptr(unsafe.Pointer(&w)),
	); e != 0 { // type syscall.Errno
		// ioctl() isn’t guaranteed to be able to request the window size on all systems.
		// The strategy is to position the cursor at the bottom-right of the
		// screen, then use escape sequences that let us query the position
		// of the cursor. That tells us how many rows and columns there must be
		// on the screen.
		io.WriteString(termOut, "\x1b[999C\x1b[999B\x1b[6n")
		var buffer [1]byte
		var buf []byte
		for cc, _ := os.Stdin.Read(buffer[:]); cc == 1; cc, _ = os.Stdin.Read(buffer[:]) {
			if buffer[0] == 'R' {
				break
			}
			buf = append(buf, buffer[0])
		}
		if string(buf[0:2]) != "\x1b[" {
			return 0, 0, fmt.Errorf("Failed to read rows;cols from tty\n")
		}
		n, e := fmt.Sscanf(string(buf[2:]), "%d;%d", &rows, &cols)
		if e != nil {
			return 0, 0, fmt.Errorf("getCursorPosition: fmt.Sscanf() failed: %s\n", e)
		}
		if n != 2 {
			return 0, 0, fmt.Errorf("getCursorPosition: got %d items, wanted 2\n", n)
		}
		return
	}
	return int(w.Row), int(w.Col), nil
}

func (c Console) editorReadKey() (outKey int) {
	defer func() {
		if *key.store {
			// only for debugging
			path := key.filename

			content, err := ioutil.ReadFile(path)
			if err != nil {
				log.Fatal(err)
			}
			content = append(content, []byte(strconv.Itoa(outKey)+" \n")...)
			err = ioutil.WriteFile(path, content, 0644)
			if err != nil {
				log.Fatal(err)
			}
		}
	}()

	var buffer [1]byte
	var cc int
	var err error
	for cc, err = os.Stdin.Read(buffer[:]); cc != 1; cc, err = os.Stdin.Read(buffer[:]) {
	}
	if err != nil {
		die(err)
	}
	if buffer[0] == '\x1b' {
		var seq [2]byte
		if cc, _ = os.Stdin.Read(seq[:]); cc != 2 {
			return '\x1b'
		}

		if seq[0] == '[' {
			if seq[1] >= '0' && seq[1] <= '9' {
				if cc, err = os.Stdin.Read(buffer[:]); cc != 1 {
					return '\x1b'
				}
				if buffer[0] == '~' {
					switch seq[1] {
					case '1':
						return HOME_KEY
					case '3':
						return DEL_KEY
					case '4':
						return END_KEY
					case '5':
						return PAGE_UP
					case '6':
						return PAGE_DOWN
					case '7':
						return HOME_KEY
					case '8':
						return END_KEY
					}
				}
				// XXX - what happens here?
			} else {
				switch seq[1] {
				case 'A':
					return ARROW_UP
				case 'B':
					return ARROW_DOWN
				case 'C':
					return ARROW_RIGHT
				case 'D':
					return ARROW_LEFT
				case 'H':
					return HOME_KEY
				case 'F':
					return END_KEY
				}
			}
		} else if seq[0] == '0' {
			switch seq[1] {
			case 'H':
				return HOME_KEY
			case 'F':
				return END_KEY
			}
		}

		return '\x1b'
	}
	return int(buffer[0])
}

// defines

const KILO_VERSION = "0.0.1"
const KILO_TAB_STOP = 8
const KILO_QUIT_TIMES = 3
const (
	BACKSPACE  = 127
	ARROW_LEFT = 1000 + iota
	ARROW_RIGHT
	ARROW_UP
	ARROW_DOWN
	DEL_KEY
	HOME_KEY
	END_KEY
	PAGE_UP
	PAGE_DOWN
)

const (
	HL_NORMAL    = 0
	HL_COMMENT   = iota
	HL_MLCOMMENT = iota
	HL_KEYWORD1  = iota
	HL_KEYWORD2  = iota
	HL_STRING    = iota
	HL_NUMBER    = iota
	HL_MATCH     = iota
)

const (
	HL_HIGHLIGHT_NUMBERS = 1 << 0
	HL_HIGHLIGHT_STRINGS = 1 << iota
)

// data

type editorSyntax struct {
	filetype               string
	filematch              []string
	keywords               []string
	singleLineCommentStart []byte
	multiLineCommentStart  []byte
	multiLineCommentEnd    []byte
	flags                  int
}

type erow struct {
	idx           int
	size          int
	rsize         int
	chars         []byte
	render        []byte
	hl            []byte
	hlOpenComment bool
}

type editorConfig struct {
	cursor   struct{ x, y int }
	rx       int
	offset   struct{ row, col int }
	screen   struct{ rows, cols int }
	rows     []erow
	dirty    bool
	filename string
	status   struct {
		msg      string
		msg_time time.Time
	}
	syntax *editorSyntax
}

var E editorConfig

// filetypes

var HLDB []editorSyntax = []editorSyntax{
	{
		filetype:  "c",
		filematch: []string{".c", ".h", ".cpp"},
		keywords: []string{"switch", "if", "while", "for",
			"break", "continue", "return", "else", "struct",
			"union", "typedef", "static", "enum", "class", "case",
			"int|", "long|", "double|", "float|", "char|",
			"unsigned|", "signed|", "void|",
		},
		singleLineCommentStart: []byte{'/', '/'},
		multiLineCommentStart:  []byte{'/', '*'},
		multiLineCommentEnd:    []byte{'*', '/'},
		flags:                  HL_HIGHLIGHT_NUMBERS | HL_HIGHLIGHT_STRINGS,
	},
}

// terminal

func die(err error) {
	// term.disableRawMode()
	io.WriteString(termOut, "\x1b[2J")
	io.WriteString(termOut, "\x1b[H")
	log.Fatal(err)
}

func TcSetAttr(fd uintptr, termios *syscall.Termios) error {
	// TCSETS+1 == TCSETSW, because TCSAFLUSH doesn't exist
	if _, _, err := syscall.Syscall(
		syscall.SYS_IOCTL,
		fd,
		uintptr(syscall.TCSETS+1), uintptr(unsafe.Pointer(termios))); err != 0 {

		return fmt.Errorf("TcSetAttr: %v", err)
	}
	return nil
}

func TcGetAttr(fd uintptr) *syscall.Termios {
	var termios = &syscall.Termios{}
	if _, _, err := syscall.Syscall(
		syscall.SYS_IOCTL,
		fd,
		syscall.TCGETS,
		uintptr(unsafe.Pointer(termios))); err != 0 {

		log.Fatalf("Problem getting terminal attributes: %s\n", err)
		return nil
	}
	return termios
}

// syntax hightlighting
var separators []byte = []byte(",.()+-/*=~%<>[]; \t\n\r")

func isSeparator(c byte) bool {
	if bytes.IndexByte(separators, c) >= 0 {
		return true
	}
	return false
}

func editorUpdateSyntax(row *erow) {
	row.hl = make([]byte, row.rsize)
	if E.syntax == nil {
		return
	}
	keywords := E.syntax.keywords[:]
	scs := E.syntax.singleLineCommentStart
	mcs := E.syntax.multiLineCommentStart
	mce := E.syntax.multiLineCommentEnd
	prevSep := true
	inComment := row.idx > 0 && E.rows[row.idx-1].hlOpenComment
	var inString byte = 0
	var skip = 0
	for i, c := range row.render {
		if skip > 0 {
			skip--
			continue
		}
		if inString == 0 && len(scs) > 0 && !inComment {
			if bytes.HasPrefix(row.render[i:], scs) {
				for j := i; j < row.rsize; j++ {
					row.hl[j] = HL_COMMENT
				}
				break
			}
		}
		if inString == 0 && len(mcs) > 0 && len(mce) > 0 {
			if inComment {
				row.hl[i] = HL_MLCOMMENT
				if bytes.HasPrefix(row.render[i:], mce) {
					for l := i; l < i+len(mce); l++ {
						row.hl[l] = HL_MLCOMMENT
					}
					skip = len(mce)
					inComment = false
					prevSep = true
				}
				continue
			} else if bytes.HasPrefix(row.render[i:], mcs) {
				for l := i; l < i+len(mcs); l++ {
					row.hl[l] = HL_MLCOMMENT
				}
				inComment = true
				skip = len(mcs)
			}
		}
		var prevHl byte = HL_NORMAL
		if i > 0 {
			prevHl = row.hl[i-1]
		}
		if (E.syntax.flags & HL_HIGHLIGHT_STRINGS) == HL_HIGHLIGHT_STRINGS {
			if inString != 0 {
				row.hl[i] = HL_STRING
				if c == '\\' && i+1 < row.rsize {
					row.hl[i+1] = HL_STRING
					skip = 1
					continue
				}
				if c == inString {
					inString = 0
				}
				prevSep = true
				continue
			} else {
				if c == '"' || c == '\'' {
					inString = c
					row.hl[i] = HL_STRING
					continue
				}
			}
		}
		if (E.syntax.flags & HL_HIGHLIGHT_NUMBERS) == HL_HIGHLIGHT_NUMBERS {
			if unicode.IsDigit(rune(c)) &&
				(prevSep || prevHl == HL_NUMBER) ||
				(c == '.' && prevHl == HL_NUMBER) {
				row.hl[i] = HL_NUMBER
				prevSep = false
				continue
			}
		}
		if prevSep {
			var j int
			var skw string
			for j, skw = range keywords {
				kw := []byte(skw)
				var color byte = HL_KEYWORD1
				idx := bytes.LastIndexByte(kw, '|')
				if idx > 0 {
					kw = kw[:idx]
					color = HL_KEYWORD2
				}
				klen := len(kw)
				if bytes.HasPrefix(row.render[i:], kw) &&
					(len(row.render[i:]) == klen ||
						isSeparator(row.render[i+klen])) {
					for l := i; l < i+klen; l++ {
						row.hl[l] = color
					}
					skip = klen - 1
					break
				}
			}
			if j < len(keywords)-1 {
				prevSep = false
				continue
			}
		}
		prevSep = isSeparator(c)
	}

	changed := row.hlOpenComment != inComment
	row.hlOpenComment = inComment
	if changed && row.idx+1 < len(E.rows) {
		editorUpdateSyntax(&E.rows[row.idx+1])
	}
}

func editorSyntaxToColor(hl byte) int {
	switch hl {
	case HL_COMMENT, HL_MLCOMMENT:
		return 36
	case HL_KEYWORD1:
		return 32
	case HL_KEYWORD2:
		return 33
	case HL_STRING:
		return 35
	case HL_NUMBER:
		return 31
	case HL_MATCH:
		return 34
	}
	return 37
}

func editorSelectSyntaxHighlight() {
	if E.filename == "" {
		return
	}

	for _, s := range HLDB {
		for _, suffix := range s.filematch {
			if strings.HasSuffix(E.filename, suffix) {
				E.syntax = &s
				return
			}
		}
	}
}

// row operations

func editorRowCxToRx(row *erow, cx int) int {
	rx := 0
	for j := 0; j < row.size && j < cx; j++ {
		if row.chars[j] == '\t' {
			rx += ((KILO_TAB_STOP - 1) - (rx % KILO_TAB_STOP))
		}
		rx++
	}
	return rx
}

func editorRowRxToCx(row *erow, rx int) int {
	curRx := 0
	var cx int
	for cx = 0; cx < row.size; cx++ {
		if row.chars[cx] == '\t' {
			curRx += (KILO_TAB_STOP - 1) - (curRx % KILO_TAB_STOP)
		}
		curRx++
		if curRx > rx {
			break
		}
	}
	return cx
}

func editorUpdateRow(row *erow) {
	tabs := 0
	for _, c := range row.chars {
		if c == '\t' {
			tabs++
		}
	}

	row.render = make([]byte, row.size+tabs*(KILO_TAB_STOP-1))

	idx := 0
	for _, c := range row.chars {
		if c == '\t' {
			row.render[idx] = ' '
			idx++
			for (idx % KILO_TAB_STOP) != 0 {
				row.render[idx] = ' '
				idx++
			}
		} else {
			row.render[idx] = c
			idx++
		}
	}
	row.rsize = idx
	editorUpdateSyntax(row)
}

func editorInsertRow(at int, s []byte) {
	if at < 0 || at > len(E.rows) {
		return
	}
	var r erow
	r.chars = s
	r.size = len(s)
	r.idx = at

	if at == 0 {
		t := make([]erow, 1)
		t[0] = r
		E.rows = append(t, E.rows...)
	} else if at == len(E.rows) {
		E.rows = append(E.rows, r)
	} else {
		t := make([]erow, 1)
		t[0] = r
		E.rows = append(E.rows[:at], append(t, E.rows[at:]...)...)
	}

	for j := at + 1; j < len(E.rows); j++ {
		E.rows[j].idx++
	}

	editorUpdateRow(&E.rows[at])
	E.dirty = true
}

func editorDelRow(at int) {
	if at < 0 || at > len(E.rows) {
		return
	}
	E.rows = append(E.rows[:at], E.rows[at+1:]...)
	E.dirty = true
	for j := at; j < len(E.rows); j++ {
		E.rows[j].idx--
	}
}

func editorRowInsertChar(row *erow, at int, c byte) {
	if at < 0 || at > row.size {
		row.chars = append(row.chars, c)
	} else if at == 0 {
		t := make([]byte, row.size+1)
		t[0] = c
		copy(t[1:], row.chars)
		row.chars = t
	} else {
		row.chars = append(
			row.chars[:at],
			append(append(make([]byte, 0), c), row.chars[at:]...)...,
		)
	}
	row.size = len(row.chars)
	editorUpdateRow(row)
	E.dirty = true
}

func editorRowAppendString(row *erow, s []byte) {
	row.chars = append(row.chars, s...)
	row.size = len(row.chars)
	editorUpdateRow(row)
	E.dirty = true
}

func editorRowDelChar(row *erow, at int) {
	if at < 0 || at > row.size {
		return
	}
	row.chars = append(row.chars[:at], row.chars[at+1:]...)
	row.size--
	E.dirty = true
	editorUpdateRow(row)
}

// editor operations

func editorInsertChar(c byte) {
	if E.cursor.y == len(E.rows) {
		var emptyRow []byte
		editorInsertRow(len(E.rows), emptyRow)
	}
	editorRowInsertChar(&E.rows[E.cursor.y], E.cursor.x, c)
	E.cursor.x++
}

func editorInsertNewLine() {
	if E.cursor.x == 0 {
		editorInsertRow(E.cursor.y, make([]byte, 0))
	} else {
		editorInsertRow(E.cursor.y+1, E.rows[E.cursor.y].chars[E.cursor.x:])
		E.rows[E.cursor.y].chars = E.rows[E.cursor.y].chars[:E.cursor.x]
		E.rows[E.cursor.y].size = len(E.rows[E.cursor.y].chars)
		editorUpdateRow(&E.rows[E.cursor.y])
	}
	E.cursor.y++
	E.cursor.x = 0
}

func editorDelChar() {
	if E.cursor.y == len(E.rows) {
		return
	}
	if E.cursor.x == 0 && E.cursor.y == 0 {
		return
	}
	if E.cursor.x > 0 {
		editorRowDelChar(&E.rows[E.cursor.y], E.cursor.x-1)
		E.cursor.x--
	} else {
		E.cursor.x = E.rows[E.cursor.y-1].size
		editorRowAppendString(&E.rows[E.cursor.y-1], E.rows[E.cursor.y].chars)
		editorDelRow(E.cursor.y)
		E.cursor.y--
	}
}

// file I/O

func editorRowsToString() (string, int) {
	totlen := 0
	buf := ""
	for _, row := range E.rows {
		totlen += row.size + 1
		buf += string(row.chars) + "\n"
	}
	return buf, totlen
}

func editorOpen(filename string) {
	E.filename = filename
	editorSelectSyntaxHighlight()
	fd, err := os.Open(filename)
	if err != nil {
		die(err)
	}
	defer fd.Close()
	fp := bufio.NewReader(fd)

	for line, err := fp.ReadBytes('\n'); err == nil; line, err = fp.ReadBytes('\n') {
		// Trim trailing newlines and carriage returns
		for c := line[len(line)-1]; len(line) > 0 && (c == '\n' || c == '\r'); {
			line = line[:len(line)-1]
			if len(line) > 0 {
				c = line[len(line)-1]
			}
		}
		editorInsertRow(len(E.rows), line)
	}

	if err != nil && err != io.EOF {
		die(err)
	}
	E.dirty = false
}

func editorSave() (err error) {
	if E.filename == "" {
		E.filename, err = editorPrompt("Save as: %q", nil)
		if err != nil {
			return fmt.Errorf("Cannot save : %v", err)
		}
		if E.filename == "" {
			editorSetStatusMessage("Save aborted")
			return
		}
		editorSelectSyntaxHighlight()
	}
	buf, len := editorRowsToString()
	fp, e := os.OpenFile(E.filename, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
	if e != nil {
		editorSetStatusMessage("Can't save! file open error %s", e)
		return
	}
	defer fp.Close()
	n, err := io.WriteString(fp, buf)
	if err == nil {
		if n == len {
			E.dirty = false
			editorSetStatusMessage("%d bytes written to disk", len)
		} else {
			editorSetStatusMessage("wanted to write %d bytes to file, wrote %d", len, n)
		}
		return
	}
	editorSetStatusMessage("Can't save! I/O error %s", err)
	return nil
}

// find

var lastMatch int = -1
var direction int = 1
var savedHlLine int
var savedHl []byte

func editorFindCallback(qry []byte, key int) {

	if savedHlLine > 0 {
		copy(E.rows[savedHlLine].hl, savedHl)
		savedHlLine = 0
		savedHl = nil
	}

	if key == '\r' || key == '\x1b' {
		lastMatch = -1
		direction = 1
		return
	} else if key == ARROW_RIGHT || key == ARROW_DOWN {
		direction = 1
	} else if key == ARROW_LEFT || key == ARROW_UP {
		direction = -1
	} else {
		lastMatch = -1
		direction = 1
	}

	if lastMatch == -1 {
		direction = 1
	}
	current := lastMatch

	for range E.rows {
		current += direction
		if current == -1 {
			current = len(E.rows) - 1
		} else if current == len(E.rows) {
			current = 0
		}
		row := &E.rows[current]
		x := bytes.Index(row.render, qry)
		if x > -1 {
			lastMatch = current
			E.cursor.y = current
			E.cursor.x = editorRowRxToCx(row, x)
			E.offset.row = len(E.rows)
			savedHlLine = current
			savedHl = make([]byte, row.rsize)
			copy(savedHl, row.hl)
			max := x + len(qry)
			for i := x; i < max; i++ {
				row.hl[i] = HL_MATCH
			}
			break
		}
	}
}

func editorFind() error {
	savedCx, savedCy := E.cursor.x, E.cursor.y
	savedColoff, savedRowoff := E.offset.col, E.offset.row
	query, err := editorPrompt("Search: %s (ESC/Arrows/Enter)", editorFindCallback)
	if err != nil {
		return fmt.Errorf("Find error: %v", err)
	}
	if query == "" {
		E.cursor.x, E.cursor.y = savedCx, savedCy
		E.offset.col, E.offset.row = savedColoff, savedRowoff
	}
	return nil
}

// input

func editorPrompt(prompt string, callback func([]byte, int)) (string, error) {
	var buf []byte

	for {
		editorSetStatusMessage(prompt, buf)
		if err := editorRefreshScreen(); err != nil {
			return "", err
		}

		c := term.editorReadKey()
		switch c {
		case DEL_KEY, ('h' & 0x1f), BACKSPACE:
			if len(buf) > 0 {
				buf = buf[:len(buf)-1]
			}
		case '\x1b':
			editorSetStatusMessage("")
			if callback != nil {
				callback(buf, c)
			}
			return "", nil
		case '\r':
			if len(buf) != 0 {
				editorSetStatusMessage("")
				if callback != nil {
					callback(buf, c)
				}
				return string(buf), nil
			}
		default:
			if unicode.IsPrint(rune(c)) {
				buf = append(buf, byte(c))
			}
		}

		if callback != nil {
			callback(buf, c)
		}
	}
}

func editorMoveCursor(key int) {
	switch key {
	case ARROW_LEFT:
		if E.cursor.x != 0 {
			E.cursor.x--
		} else if E.cursor.y > 0 {
			E.cursor.y--
			E.cursor.x = E.rows[E.cursor.y].size
		}
	case ARROW_RIGHT:
		if E.cursor.y < len(E.rows) {
			if E.cursor.x < E.rows[E.cursor.y].size {
				E.cursor.x++
			} else if E.cursor.x == E.rows[E.cursor.y].size {
				E.cursor.y++
				E.cursor.x = 0
			}
		}
	case ARROW_UP:
		if E.cursor.y != 0 {
			E.cursor.y--
		}
	case ARROW_DOWN:
		if E.cursor.y < len(E.rows) {
			E.cursor.y++
		}
	}

	rowlen := 0
	if E.cursor.y < len(E.rows) {
		rowlen = E.rows[E.cursor.y].size
	}
	if E.cursor.x > rowlen {
		E.cursor.x = rowlen
	}
}

var quitTimes int = KILO_QUIT_TIMES

func editorProcessKeypress() (outOfProgram bool) {
	c := term.editorReadKey()
	switch c {
	case '\r':
		editorInsertNewLine()
		break
	case ('q' & 0x1f):
		if E.dirty && quitTimes > 0 {
			editorSetStatusMessage("Warning!!! File has unsaved changes. Press Ctrl-Q %d more times to quit.", quitTimes)
			quitTimes--
			return
		}
		io.WriteString(termOut, "\x1b[2J")
		io.WriteString(termOut, "\x1b[H")
		// term.disableRawMode()
		// os.Exit(0)
		return true
	case ('s' & 0x1f):
		editorSave()
	case HOME_KEY:
		E.cursor.x = 0
	case END_KEY:
		if E.cursor.y < len(E.rows) {
			E.cursor.x = E.rows[E.cursor.y].size
		}
	case ('f' & 0x1f):
		editorFind()
	case ('h' & 0x1f), BACKSPACE, DEL_KEY:
		if c == DEL_KEY {
			editorMoveCursor(ARROW_RIGHT)
		}
		editorDelChar()
		break
	case PAGE_UP, PAGE_DOWN:
		dir := ARROW_DOWN
		if c == PAGE_UP {
			E.cursor.y = E.offset.row
			dir = ARROW_UP
		} else {
			E.cursor.y = E.offset.row + E.screen.rows - 1
			if E.cursor.y > len(E.rows) {
				E.cursor.y = len(E.rows)
			}
		}
		for times := E.screen.rows; times > 0; times-- {
			editorMoveCursor(dir)
		}
	case ARROW_UP, ARROW_DOWN, ARROW_LEFT, ARROW_RIGHT:
		editorMoveCursor(c)
	case ('l' & 0x1f):
		break
	case '\x1b':
		break
	default:
		editorInsertChar(byte(c))
	}
	quitTimes = KILO_QUIT_TIMES
	return
}

// output

func editorScroll() {
	E.rx = 0

	if E.cursor.y < len(E.rows) {
		E.rx = editorRowCxToRx(&(E.rows[E.cursor.y]), E.cursor.x)
	}

	if E.cursor.y < E.offset.row {
		E.offset.row = E.cursor.y
	}
	if E.cursor.y >= E.offset.row+E.screen.rows {
		E.offset.row = E.cursor.y - E.screen.rows + 1
	}
	if E.rx < E.offset.col {
		E.offset.col = E.rx
	}
	if E.rx >= E.offset.col+E.screen.cols {
		E.offset.col = E.rx - E.screen.cols + 1
	}
}

func editorRefreshScreen() error {
	editorScroll()
	ab := bytes.NewBufferString("\x1b[25l")
	ab.WriteString("\x1b[H")
	editorDrawRows(ab)
	editorDrawStatusBar(ab)
	editorDrawMessageBar(ab)
	ab.WriteString(fmt.Sprintf("\x1b[%d;%dH", (E.cursor.y-E.offset.row)+1, (E.rx-E.offset.col)+1))
	ab.WriteString("\x1b[?25h")
	_, err := ab.WriteTo(termOut)
	if err != nil {
		return fmt.Errorf("Cannot refresh screen : %v", err)
	}
	return nil
}

func editorDrawRows(ab *bytes.Buffer) {
	for y := 0; y < E.screen.rows; y++ {
		filerow := y + E.offset.row
		if filerow >= len(E.rows) {
			if len(E.rows) == 0 && y == E.screen.rows/3 {
				w := fmt.Sprintf("Kilo editor -- version %s", KILO_VERSION)
				if len(w) > E.screen.cols {
					w = w[0:E.screen.cols]
				}
				pad := "~ "
				for padding := (E.screen.cols - len(w)) / 2; padding > 0; padding-- {
					ab.WriteString(pad)
					pad = " "
				}
				ab.WriteString(w)
			} else {
				ab.WriteString("~")
			}
		} else {
			len := E.rows[filerow].rsize - E.offset.col
			if len < 0 {
				len = 0
			}
			if len > 0 {
				if len > E.screen.cols {
					len = E.screen.cols
				}
				rindex := E.offset.col + len
				hl := E.rows[filerow].hl[E.offset.col:rindex]
				currentColor := -1
				for j, c := range E.rows[filerow].render[E.offset.col:rindex] {
					if unicode.IsControl(rune(c)) {
						ab.WriteString("\x1b[7m")
						if c < 26 {
							ab.WriteString("@")
						} else {
							ab.WriteString("?")
						}
						ab.WriteString("\x1b[m")
						if currentColor != -1 {
							ab.WriteString(fmt.Sprintf("\x1b[%dm", currentColor))
						}
					} else if hl[j] == HL_NORMAL {
						if currentColor != -1 {
							ab.WriteString("\x1b[39m")
							currentColor = -1
						}
						ab.WriteByte(c)
					} else {
						color := editorSyntaxToColor(hl[j])
						if color != currentColor {
							currentColor = color
							buf := fmt.Sprintf("\x1b[%dm", color)
							ab.WriteString(buf)
						}
						ab.WriteByte(c)
					}
				}
				ab.WriteString("\x1b[39m")
			}
		}
		ab.WriteString("\x1b[K")
		ab.WriteString("\r\n")
	}
}

func editorDrawStatusBar(ab *bytes.Buffer) {
	ab.WriteString("\x1b[7m")
	fname := E.filename
	if fname == "" {
		fname = "[No Name]"
	}
	modified := ""
	if E.dirty {
		modified = "(modified)"
	}
	status := fmt.Sprintf("%.20s - %d lines %s", fname, len(E.rows), modified)
	ln := len(status)
	if ln > E.screen.cols {
		ln = E.screen.cols
	}
	filetype := "no ft"
	if E.syntax != nil {
		filetype = E.syntax.filetype
	}
	rstatus := fmt.Sprintf("%s | %d/%d", filetype, E.cursor.y+1, len(E.rows))
	rlen := len(rstatus)
	ab.WriteString(status[:ln])
	for ln < E.screen.cols {
		if E.screen.cols-ln == rlen {
			ab.WriteString(rstatus)
			break
		} else {
			ab.WriteString(" ")
			ln++
		}
	}
	ab.WriteString("\x1b[m")
	ab.WriteString("\r\n")
}

func editorDrawMessageBar(ab *bytes.Buffer) {
	ab.WriteString("\x1b[K")
	msglen := len(E.status.msg)
	if msglen > E.screen.cols {
		msglen = E.screen.cols
	}
	if msglen > 0 && (time.Now().Sub(E.status.msg_time) < 5*time.Second) {
		ab.WriteString(E.status.msg)
	}
}

func editorSetStatusMessage(format string, a ...interface{}) {
	E.status.msg = fmt.Sprintf(format, a...)
	E.status.msg_time = time.Now()
}

// init

// flags
var key = struct {
	store    *bool
	filename string // path of keys filename
	text     string // path of text filename
}{}

func main() {
	// flag
	key.store = flag.Bool("kr", false, "Debug tool for keys record and save file result.\n"+
		"Files(keys, text) are save in folder './testdata/'.")
	filename := flag.String("e", "", "Edit file")

	flag.Parse()

	E.filename = *filename

	// generate key store
	if *key.store {
		for prefix := 0; ; prefix++ {
			key.filename = fmt.Sprintf("./testdata/%d.keys", prefix)
			key.text = fmt.Sprintf("./testdata/%d.file", prefix)
			if _, err := os.Stat(key.filename); os.IsNotExist(err) { // create file if not exists
				files := []string{key.filename, key.text}
				for _, filename := range files {
					file, err := os.Create(filename)
					if err != nil {
						log.Fatal(err)
						return
					}
					file.Close()
				}
				break
			}
		}
	}

	//  enable raw mode
	origTermios := TcGetAttr(os.Stdin.Fd())
	var raw syscall.Termios
	raw = *origTermios
	raw.Iflag &^= syscall.BRKINT | syscall.ICRNL | syscall.INPCK | syscall.ISTRIP | syscall.IXON
	raw.Oflag &^= syscall.OPOST
	raw.Cflag |= syscall.CS8
	raw.Lflag &^= syscall.ECHO | syscall.ICANON | syscall.IEXTEN | syscall.ISIG
	raw.Cc[syscall.VMIN+1] = 0
	raw.Cc[syscall.VTIME+1] = 1
	if e := TcSetAttr(os.Stdin.Fd(), &raw); e != nil {
		log.Fatalf("Problem enabling raw mode: %s\n", e)
	}

	defer func() {
		// disable raw mode
		if e := TcSetAttr(os.Stdin.Fd(), origTermios); e != nil {
			log.Fatalf("Problem disabling raw mode: %s\n", e)
		}
	}()

	err := run()
	if err != nil {
		log.Fatal(err)
	}
}

func initEditor() (err error) {
	// Initialization a la C not necessary.
	if E.screen.rows, E.screen.cols, err = term.getWindowSize(); err != nil {
		return fmt.Errorf("couldn't get screen size: %v", err)
	}
	E.screen.rows -= 2
	return nil
}

func run() error {
	err := initEditor()
	if err != nil {
		return fmt.Errorf("Cannot initialize editor: %v", err)
	}
	if key.store != nil && *key.store {
		editorOpen(key.text)
	} else if E.filename != "" {
		editorOpen(E.filename)
	}

	editorSetStatusMessage("HELP: Ctrl-S = save | Ctrl-Q = quit | Ctrl-F = find")

	for {
		if err := editorRefreshScreen(); err != nil {
			return err
		}
		if editorProcessKeypress() {
			// if enable close key
			break
		}
	}
	return nil
}
