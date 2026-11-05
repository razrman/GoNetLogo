package nl

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
)

type fileHandle struct {
	path    string
	f       *os.File
	writing bool
	reader  *bufio.Reader
	atEnd   bool
}

type fileMgr struct {
	current *fileHandle
	open    map[string]*fileHandle
}

func (vm *VM) fileMgr() *fileMgr {
	if vm.files == nil {
		vm.files = &fileMgr{open: map[string]*fileHandle{}}
	}
	return vm.files
}

func (vm *VM) resolvePath(p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	if vm.ModelDir != "" {
		return filepath.Join(vm.ModelDir, p)
	}
	return p
}

func (vm *VM) fileOpen(path string) error {
	mgr := vm.fileMgr()
	fullPath := vm.resolvePath(path)
	if h, ok := mgr.open[fullPath]; ok {
		mgr.current = h
		return nil
	}
	f, err := os.OpenFile(fullPath, os.O_RDWR|os.O_CREATE, 0666)
	if err != nil {
		return err
	}
	h := &fileHandle{
		path:   fullPath,
		f:      f,
		reader: bufio.NewReader(f),
	}
	mgr.open[fullPath] = h
	mgr.current = h
	return nil
}

func (vm *VM) fileClose() error {
	mgr := vm.fileMgr()
	if mgr.current == nil {
		return nil
	}
	h := mgr.current
	delete(mgr.open, h.path)
	mgr.current = nil
	return h.f.Close()
}

func (vm *VM) fileCloseAll() error {
	mgr := vm.fileMgr()
	var firstErr error
	for p, h := range mgr.open {
		if err := h.f.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		delete(mgr.open, p)
	}
	mgr.current = nil
	return firstErr
}

func (vm *VM) fileDelete(path string) error {
	mgr := vm.fileMgr()
	fullPath := vm.resolvePath(path)
	if h, ok := mgr.open[fullPath]; ok {
		_ = h.f.Close()
		delete(mgr.open, fullPath)
		if mgr.current == h {
			mgr.current = nil
		}
	}
	return os.Remove(fullPath)
}

func (vm *VM) fileExists(path string) bool {
	fullPath := vm.resolvePath(path)
	_, err := os.Stat(fullPath)
	return err == nil
}

func (vm *VM) fileFlush() error {
	mgr := vm.fileMgr()
	if mgr.current == nil {
		return fmt.Errorf("no open file to flush")
	}
	return mgr.current.f.Sync()
}

func (vm *VM) ensureWriting() error {
	mgr := vm.fileMgr()
	if mgr.current == nil {
		return fmt.Errorf("no open file")
	}
	h := mgr.current
	if !h.writing {
		if _, err := h.f.Seek(0, io.SeekEnd); err != nil {
			return err
		}
		h.writing = true
		h.reader = nil
	}
	return nil
}

func (vm *VM) filePrint(v Value) error {
	if err := vm.ensureWriting(); err != nil {
		return err
	}
	_, err := fmt.Fprintf(vm.fileMgr().current.f, "%s\n", v.Format(false))
	return err
}

func (vm *VM) fileType(v Value) error {
	if err := vm.ensureWriting(); err != nil {
		return err
	}
	_, err := fmt.Fprintf(vm.fileMgr().current.f, "%s", v.Format(false))
	return err
}

func (vm *VM) fileShow(v Value) error {
	if err := vm.ensureWriting(); err != nil {
		return err
	}
	prefix := ""
	if vm.self != nil {
		prefix = vm.self.AgentString() + ": "
	}
	_, err := fmt.Fprintf(vm.fileMgr().current.f, "%s%s\n", prefix, v.Format(true))
	return err
}

func (vm *VM) fileWrite(v Value) error {
	if err := vm.ensureWriting(); err != nil {
		return err
	}
	_, err := fmt.Fprintf(vm.fileMgr().current.f, " %s", v.Format(true))
	return err
}

func (vm *VM) fileAtEnd() (bool, error) {
	mgr := vm.fileMgr()
	if mgr.current == nil {
		return false, fmt.Errorf("no open file")
	}
	h := mgr.current
	if h.atEnd {
		return true, nil
	}
	if h.reader == nil {
		h.reader = bufio.NewReader(h.f)
	}
	for {
		b, err := h.reader.Peek(1)
		if err != nil {
			if err == io.EOF {
				h.atEnd = true
				return true, nil
			}
			return false, err
		}
		if len(b) > 0 {
			return false, nil
		}
	}
}

func (vm *VM) fileReadLine() (Value, error) {
	mgr := vm.fileMgr()
	if mgr.current == nil {
		return Value{}, fmt.Errorf("no open file")
	}
	h := mgr.current
	if h.reader == nil {
		h.reader = bufio.NewReader(h.f)
	}
	line, err := h.reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return Value{}, err
	}
	if err == io.EOF {
		h.atEnd = true
		if len(line) == 0 {
			return Value{}, fmt.Errorf("end of file reached")
		}
	}
	line = strings.TrimRight(line, "\r\n")
	return Str(line), nil
}

func (vm *VM) readingHandle() (*fileHandle, error) {
	mgr := vm.fileMgr()
	if mgr.current == nil {
		return nil, fmt.Errorf("no open file")
	}
	h := mgr.current
	if h.reader == nil {
		h.reader = bufio.NewReader(h.f)
	}
	return h, nil
}

// fileRead reads one NetLogo constant: a number, string, boolean, nobody,
// or a list of those. It does not evaluate reporters or read globals.
func (vm *VM) fileRead() (Value, error) {
	h, err := vm.readingHandle()
	if err != nil {
		return Value{}, err
	}
	v, err := readFileValue(h)
	if err == io.EOF {
		h.atEnd = true
		return Value{}, fmt.Errorf("end of file reached")
	}
	return v, err
}

func (vm *VM) fileReadChars(n int) (Value, error) {
	if n < 0 {
		return Value{}, fmt.Errorf("file-read-characters expected a non-negative number")
	}
	h, err := vm.readingHandle()
	if err != nil {
		return Value{}, err
	}
	if n == 0 {
		return Str(""), nil
	}
	var b strings.Builder
	for i := 0; i < n; i++ {
		r, _, err := h.reader.ReadRune()
		if err != nil {
			h.atEnd = true
			if err == io.EOF {
				return Value{}, fmt.Errorf("end of file reached")
			}
			return Value{}, err
		}
		b.WriteRune(r)
	}
	return Str(b.String()), nil
}

func (vm *VM) userPick(fn func() (string, bool)) (Value, error) {
	if fn == nil {
		return Bool(false), nil
	}
	path, ok := fn()
	if !ok || path == "" {
		return Bool(false), nil
	}
	return Str(path), nil
}

func readFileValue(h *fileHandle) (Value, error) {
	if err := skipFileSpace(h); err != nil {
		return Value{}, err
	}
	r, _, err := h.reader.ReadRune()
	if err != nil {
		return Value{}, err
	}
	switch r {
	case '[':
		return readFileList(h)
	case '"':
		return readFileString(h)
	case ']', '(', ')', '{', '}':
		return Value{}, fmt.Errorf("file-read expected a constant, got %q", string(r))
	default:
		if err := h.reader.UnreadRune(); err != nil {
			return Value{}, err
		}
		tok, err := readFileToken(h)
		if err != nil {
			return Value{}, err
		}
		return parseFileAtom(tok)
	}
}

func readFileList(h *fileHandle) (Value, error) {
	var items []Value
	for {
		if err := skipFileSpace(h); err != nil {
			if err == io.EOF {
				h.atEnd = true
				return Value{}, fmt.Errorf("unexpected end of file reading list")
			}
			return Value{}, err
		}
		r, _, err := h.reader.ReadRune()
		if err != nil {
			h.atEnd = true
			return Value{}, fmt.Errorf("unexpected end of file reading list")
		}
		if r == ']' {
			if items == nil {
				items = []Value{}
			}
			return List(items...), nil
		}
		if err := h.reader.UnreadRune(); err != nil {
			return Value{}, err
		}
		v, err := readFileValue(h)
		if err != nil {
			if err == io.EOF {
				h.atEnd = true
				return Value{}, fmt.Errorf("unexpected end of file reading list")
			}
			return Value{}, err
		}
		items = append(items, v)
	}
}

func readFileString(h *fileHandle) (Value, error) {
	var b strings.Builder
	for {
		r, _, err := h.reader.ReadRune()
		if err != nil {
			h.atEnd = true
			return Value{}, fmt.Errorf("unexpected end of file reading string")
		}
		if r == '"' {
			return Str(b.String()), nil
		}
		if r == '\\' {
			esc, _, err := h.reader.ReadRune()
			if err != nil {
				h.atEnd = true
				return Value{}, fmt.Errorf("unexpected end of file reading string")
			}
			switch esc {
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			case 'r':
				b.WriteByte('\r')
			case '\\', '"':
				b.WriteRune(esc)
			default:
				return Value{}, fmt.Errorf("file-read found an illegal character after a backslash")
			}
			continue
		}
		if r == '\n' {
			return Value{}, fmt.Errorf("file-read reached the end of the line before a closing quote")
		}
		b.WriteRune(r)
	}
}

func readFileToken(h *fileHandle) (string, error) {
	var b strings.Builder
	for {
		r, _, err := h.reader.ReadRune()
		if err != nil {
			if err == io.EOF {
				h.atEnd = true
				if b.Len() == 0 {
					return "", err
				}
				return b.String(), nil
			}
			return "", err
		}
		if unicode.IsSpace(r) || r == '[' || r == ']' || r == '"' {
			if err := h.reader.UnreadRune(); err != nil {
				return "", err
			}
			break
		}
		b.WriteRune(r)
	}
	if b.Len() == 0 {
		return "", fmt.Errorf("file-read expected a constant")
	}
	return b.String(), nil
}

func parseFileAtom(tok string) (Value, error) {
	switch strings.ToLower(tok) {
	case "true":
		return Bool(true), nil
	case "false":
		return Bool(false), nil
	case "nobody":
		return Nobody(), nil
	}
	n, err := strconv.ParseFloat(tok, 64)
	if err != nil {
		return Value{}, fmt.Errorf("file-read expected a constant, got %s", tok)
	}
	return Num(n), nil
}

func skipFileSpace(h *fileHandle) error {
	for {
		r, _, err := h.reader.ReadRune()
		if err != nil {
			return err
		}
		if !unicode.IsSpace(r) {
			return h.reader.UnreadRune()
		}
	}
}
