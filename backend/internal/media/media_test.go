package media

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
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

func TestPixelLimitBeforeDecoding(t *testing.T) {
	var data bytes.Buffer
	if err := png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	malformed := data.Bytes()
	// Valid IHDR declaring 5001² pixels; no huge decoded allocation is needed.
	binary.BigEndian.PutUint32(malformed[16:20], 5001)
	binary.BigEndian.PutUint32(malformed[20:24], 5001)
	binary.BigEndian.PutUint32(malformed[29:33], crc32.ChecksumIEEE(malformed[12:29]))
	if _, err := Save(t.TempDir(), bytes.NewReader(malformed), "image/png"); err != ErrLarge {
		t.Fatalf("pixel bound: %v", err)
	}
}
