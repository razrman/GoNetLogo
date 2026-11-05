package nl

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

type fileHandle struct {
	path     string
	f        *os.File
	writing  bool
	reader   *bufio.Reader
	atEnd    bool
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

func (vm *VM) fileRead() (Value, error) {
	mgr := vm.fileMgr()
	if mgr.current == nil {
		return Value{}, fmt.Errorf("no open file")
	}
	h := mgr.current
	if h.reader == nil {
		h.reader = bufio.NewReader(h.f)
	}
	for {
		r, _, err := h.reader.ReadRune()
		if err != nil {
			h.atEnd = true
			return Value{}, fmt.Errorf("end of file reached")
		}
		if !unicode.IsSpace(r) {
			_ = h.reader.UnreadRune()
			break
		}
	}
	r, _, err := h.reader.ReadRune()
	if err != nil {
		h.atEnd = true
		return Value{}, fmt.Errorf("end of file reached")
	}
	if r == '[' {
		depth := 1
		var buf strings.Builder
		buf.WriteRune(r)
		for depth > 0 {
			ch, _, err := h.reader.ReadRune()
			if err != nil {
				h.atEnd = true
				return Value{}, fmt.Errorf("unexpected end of file reading list")
			}
			buf.WriteRune(ch)
			if ch == '[' {
				depth++
			} else if ch == ']' {
				depth--
			}
		}
		expr, err := ParseReporter(buf.String(), vm.Prog)
		if err != nil {
			return Value{}, err
		}
		return vm.eval(expr)
	} else if r == '"' {
		var buf strings.Builder
		buf.WriteRune(r)
		for {
			ch, _, err := h.reader.ReadRune()
			if err != nil {
				h.atEnd = true
				return Value{}, fmt.Errorf("unexpected end of file reading string")
			}
			buf.WriteRune(ch)
			if ch == '"' {
				break
			}
		}
		expr, err := ParseReporter(buf.String(), vm.Prog)
		if err != nil {
			return Value{}, err
		}
		return vm.eval(expr)
	} else {
		var buf strings.Builder
		buf.WriteRune(r)
		for {
			ch, _, err := h.reader.ReadRune()
			if err != nil {
				if err == io.EOF {
					h.atEnd = true
					break
				}
				return Value{}, err
			}
			if unicode.IsSpace(ch) || ch == '[' || ch == ']' || ch == '"' {
				_ = h.reader.UnreadRune()
				break
			}
			buf.WriteRune(ch)
		}
		tokStr := buf.String()
		expr, err := ParseReporter(tokStr, vm.Prog)
		if err != nil {
			return Value{}, err
		}
		return vm.eval(expr)
	}
}
