// Package media validates and re-encodes untrusted images before storage.
package media

import (
	"bytes"
	"errors"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"

	"github.com/google/uuid"
)

const MaxBytes int64 = 10 * 1024 * 1024
const MaxPixels int64 = 25_000_000

var ErrLarge = errors.New("image too large")
var ErrFormat = errors.New("unsupported image")

type File struct {
	Key    string
	MIME   string
	Size   int64
	Width  int
	Height int
}

func Save(dir string, reader io.Reader, claimed string) (File, error) {
	var out File
	data, err := io.ReadAll(io.LimitReader(reader, MaxBytes+1))
	if err != nil {
		return out, err
	}
	if int64(len(data)) > MaxBytes {
		return out, ErrLarge
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || (format != "jpeg" && format != "png") {
		return out, ErrFormat
	}
	if claimed != "image/"+format {
		return out, ErrFormat
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width) > MaxPixels/int64(cfg.Height) {
		return out, ErrLarge
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return out, ErrFormat
	}
	key := uuid.NewString() + "." + format
	f, err := os.OpenFile(filepath.Join(dir, key), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return out, err
	}
	good := false
	defer func() {
		f.Close()
		if !good {
			os.Remove(filepath.Join(dir, key))
		}
	}()
	// Encoders copy only decoded pixels, discarding EXIF, comments and location.
	if format == "jpeg" {
		err = jpeg.Encode(f, img, &jpeg.Options{Quality: 88})
	} else {
		err = png.Encode(f, img)
	}
	if err != nil {
		return out, err
	}
	if err = f.Sync(); err != nil {
		return out, err
	}
	stat, err := f.Stat()
	if err != nil {
		return out, err
	}
	if err = f.Close(); err != nil {
		return out, err
	}
	good = true
	return File{Key: key, MIME: "image/" + format, Size: stat.Size(), Width: cfg.Width, Height: cfg.Height}, nil
}
