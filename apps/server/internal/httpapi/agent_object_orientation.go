package httpapi

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"image/jpeg"
)

// Detectors using EXIF transpose operate on display-oriented pixels. Match that
// orientation before cropping; never apply their boxes to the encoded rotation.
func normalizeObjectImage(body []byte) ([]byte, error) {
	config, format, err := image.DecodeConfig(bytes.NewReader(body))
	if err != nil || int64(config.Width)*int64(config.Height) > 64_000_000 {
		return nil, errors.New("AGENT_TOOL_IMAGE_UNSUPPORTED")
	}
	orientation := 1
	if format == "jpeg" {
		for pos := 2; pos+4 <= len(body); {
			if body[pos] != 0xff {
				break
			}
			marker := body[pos+1]
			if marker == 0xda || marker == 0xd9 {
				break
			}
			length := int(binary.BigEndian.Uint16(body[pos+2 : pos+4]))
			if length < 2 || pos+2+length > len(body) {
				break
			}
			payload := body[pos+4 : pos+2+length]
			if marker == 0xe1 && bytes.HasPrefix(payload, []byte("Exif\x00\x00")) {
				orientation = tiffObjectOrientation(payload[6:])
				break
			}
			pos += 2 + length
		}
	} else if format == "png" {
		for pos := 8; pos+12 <= len(body); {
			length := int(binary.BigEndian.Uint32(body[pos : pos+4]))
			if length < 0 || length > len(body)-pos-12 {
				break
			}
			if string(body[pos+4:pos+8]) == "eXIf" {
				orientation = tiffObjectOrientation(body[pos+8 : pos+8+length])
				break
			}
			pos += 12 + length
		}
	}
	if orientation == 1 {
		return body, nil
	}
	if orientation < 1 || orientation > 8 {
		return nil, errors.New("AGENT_TOOL_IMAGE_ORIENTATION_INVALID")
	}
	source, _, err := image.Decode(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	w, h := config.Width, config.Height
	dw, dh := w, h
	if orientation >= 5 {
		dw, dh = h, w
	}
	output := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for y := 0; y < dh; y++ {
		for x := 0; x < dw; x++ {
			sx, sy := x, y
			switch orientation {
			case 2:
				sx = w - 1 - x
			case 3:
				sx = w - 1 - x
				sy = h - 1 - y
			case 4:
				sy = h - 1 - y
			case 5:
				sx = y
				sy = x
			case 6:
				sx = y
				sy = h - 1 - x
			case 7:
				sx = w - 1 - y
				sy = h - 1 - x
			case 8:
				sx = w - 1 - y
				sy = x
			}
			output.Set(x, y, source.At(sx, sy))
		}
	}
	var encoded bytes.Buffer
	err = jpeg.Encode(&encoded, output, &jpeg.Options{Quality: 95})
	return encoded.Bytes(), err
}
func tiffObjectOrientation(tiff []byte) int {
	if len(tiff) < 8 {
		return 0
	}
	var order binary.ByteOrder
	switch string(tiff[:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return 0
	}
	if order.Uint16(tiff[2:4]) != 42 {
		return 0
	}
	offset := int(order.Uint32(tiff[4:8]))
	if offset < 8 || offset > len(tiff)-2 {
		return 0
	}
	count := int(order.Uint16(tiff[offset : offset+2]))
	offset += 2
	if count > (len(tiff)-offset)/12 {
		return 0
	}
	for n := 0; n < count; n++ {
		entry := tiff[offset+n*12 : offset+(n+1)*12]
		if order.Uint16(entry[:2]) == 0x0112 {
			if order.Uint16(entry[2:4]) != 3 || order.Uint32(entry[4:8]) != 1 {
				return 0
			}
			return int(order.Uint16(entry[8:10]))
		}
	}
	return 1
}
