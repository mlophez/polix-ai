// Package jsonl reads and appends files of one record per line, such as JSON Lines logs. Reading is bounded:
// a line over the limit is skipped as it is read and counted, so a huge line never stops the reading nor fills
// the memory.
package jsonl

import (
	"bufio"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// Read returns every line of the file at path, without its newline, and how many lines it skipped for being
// over limit bytes. A file that does not exist has no lines; it fails only when the file exists and cannot be
// read.
func Read(path string, limit int) (lines [][]byte, skipped int, err error) {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()
	err = readLines(f, limit, func(line []byte, tooLong bool) {
		if tooLong {
			skipped++
			return
		}
		lines = append(lines, append([]byte(nil), line...))
	})
	if err != nil {
		return nil, 0, err
	}
	return lines, skipped, nil
}

// Append writes line and a newline at the end of the file at path, creating it and its folder when missing.
// The line is written with one call in append mode, which keeps concurrent writers from interleaving lines in
// practice.
func Append(path string, line []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(append([]byte(nil), line...), '\n'))
	return err
}

// readLines calls fn with every line of r without its newline; a line over limit is discarded as it is read
// and reported with tooLong.
func readLines(r io.Reader, limit int, fn func(line []byte, tooLong bool)) error {
	br := bufio.NewReaderSize(r, 64*1024)
	var buf []byte
	tooLong := false
	for {
		chunk, err := br.ReadSlice('\n')
		if !tooLong {
			if len(buf)+len(chunk) > limit+1 {
				tooLong, buf = true, buf[:0]
			} else {
				buf = append(buf, chunk...)
			}
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		if err != nil && !errors.Is(err, io.EOF) {
			return err
		}
		if tooLong || len(buf) > 0 {
			line := buf
			if n := len(line); n > 0 && line[n-1] == '\n' {
				line = line[:n-1]
			}
			fn(line, tooLong)
		}
		buf, tooLong = buf[:0], false
		if errors.Is(err, io.EOF) {
			return nil
		}
	}
}
