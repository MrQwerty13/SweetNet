package media

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestReencodingAndLimits(t *testing.T) {
	dir := t.TempDir()
	var jpegData bytes.Buffer
	jpeg.Encode(&jpegData, image.NewRGBA(image.Rect(0, 0, 16, 8)), nil)
	marker := []byte("Exif\x00\x00GPS-secret-location")
	segment := []byte{0xff, 0xe1, 0, 0}
	binary.BigEndian.PutUint16(segment[2:], uint16(len(marker)+2))
	withEXIF := append([]byte{0xff, 0xd8}, segment...)
	withEXIF = append(withEXIF, marker...)
	withEXIF = append(withEXIF, jpegData.Bytes()[2:]...)
	f, err := Save(dir, bytes.NewReader(withEXIF), "image/jpeg")
	if err != nil {
		t.Fatal(err)
	}
	saved, _ := os.ReadFile(filepath.Join(dir, f.Key))
	if bytes.Contains(saved, marker) || f.Width != 16 || f.Height != 8 {
		t.Fatal("EXIF retained or wrong dimensions")
	}
	if _, err := Save(dir, bytes.NewReader(jpegData.Bytes()), "image/png"); err != ErrFormat {
		t.Fatal("fake MIME accepted")
	}
	if _, err := Save(dir, bytes.NewReader([]byte("<svg/>")), "image/png"); err != ErrFormat {
		t.Fatal("SVG accepted")
	}
	if _, err := Save(dir, bytes.NewReader(make([]byte, MaxBytes+1)), "image/png"); err != ErrLarge {
		t.Fatal("large file accepted")
	}
	var pngData bytes.Buffer
	png.Encode(&pngData, image.NewRGBA(image.Rect(0, 0, 3, 3)))
	if _, err := Save(dir, &pngData, "image/png"); err != nil {
		t.Fatal(err)
	}
	files, _ := os.ReadDir(dir)
	if len(files) != 2 {
		t.Fatalf("invalid uploads left files: %d", len(files))
	}
}
