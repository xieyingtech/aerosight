package httpapi

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/jpeg"
	"testing"
)

func TestObjectImageEXIFOrientation(t *testing.T) {
	source := image.NewRGBA(image.Rect(0, 0, 30, 10))
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, source, nil); err != nil {
		t.Fatal(err)
	}
	tiff := make([]byte, 26)
	copy(tiff, "II")
	binary.LittleEndian.PutUint16(tiff[2:], 42)
	binary.LittleEndian.PutUint32(tiff[4:], 8)
	binary.LittleEndian.PutUint16(tiff[8:], 1)
	binary.LittleEndian.PutUint16(tiff[10:], 0x0112)
	binary.LittleEndian.PutUint16(tiff[12:], 3)
	binary.LittleEndian.PutUint32(tiff[14:], 1)
	binary.LittleEndian.PutUint16(tiff[18:], 6)
	payload := append([]byte("Exif\x00\x00"), tiff...)
	segment := []byte{0xff, 0xe1, 0, byte(len(payload) + 2)}
	segment = append(segment, payload...)
	original := encoded.Bytes()
	body := append([]byte{}, original[:2]...)
	body = append(body, segment...)
	body = append(body, original[2:]...)
	normalized, err := normalizeObjectImage(body)
	if err != nil {
		t.Fatal(err)
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(normalized))
	if err != nil || config.Width != 10 || config.Height != 30 {
		t.Fatal(config, err)
	}
	photos, keys, err := objectQueryPhotos(body, []queryDetection{{DetectionKey: "rotated", Label: "person", Confidence: .9, PixelGeometry: &queryBox{Type: "bbox", X: 0, Y: 15, Width: 10, Height: 15}}})
	if err != nil || len(photos) != 2 || len(keys) != 1 {
		t.Fatal(photos, keys, err)
	}
	for _, raw := range [][]byte{nil, []byte("II")} {
		if tiffObjectOrientation(raw) != 0 {
			t.Fatal("invalid TIFF accepted")
		}
	}
}
