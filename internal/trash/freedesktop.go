package trash

import (
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/uvw/claudes/internal/files"
)

// toFreedesktop moves path into the trash at dir, as the freedesktop.org
// Trash specification lays it out: the file under files/, and a .trashinfo
// file under info/ that records where it came from and when, so a file manager
// can put it back. The info file is created first, and exclusively, to claim
// the name.
func toFreedesktop(path, dir string, now time.Time) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	stored, info := filepath.Join(dir, "files"), filepath.Join(dir, "info")
	for _, d := range []string{stored, info} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return err
		}
	}
	record := fmt.Sprintf("[Trash Info]\nPath=%s\nDeletionDate=%s\n",
		escapePath(abs), now.Format("2006-01-02T15:04:05"))
	base := filepath.Base(abs)
	for i := 1; ; i++ {
		name := base
		if i > 1 {
			name = fmt.Sprintf("%s.%d", base, i)
		}
		infoPath := filepath.Join(info, name+".trashinfo")
		f, err := os.OpenFile(infoPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return err
		}
		_, werr := f.WriteString(record)
		if cerr := f.Close(); werr == nil {
			werr = cerr
		}
		if werr == nil {
			werr = files.RenameNoReplace(abs, filepath.Join(stored, name))
			if werr == nil {
				return nil
			}
			if errors.Is(werr, fs.ErrExist) {
				os.Remove(infoPath)
				continue
			}
		}
		os.Remove(infoPath)
		return werr
	}
}

// escapePath percent-encodes a path as the specification asks, keeping the
// slashes between its parts.
func escapePath(p string) string {
	parts := strings.Split(p, "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}
