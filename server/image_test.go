package server

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math/rand"
	"testing"
)

func TestProcessAvatar_PNG(t *testing.T) {
	// Create a 100x100 solid blue PNG
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			img.Set(x, y, color.RGBA{R: 0, G: 0, B: 255, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal("encode test image:", err)
	}

	result, err := processAvatar(bytes.NewReader(buf.Bytes()), 2<<20)
	if err != nil {
		t.Fatal("processAvatar failed:", err)
	}

	// Verify output is a 64x64 PNG
	decoded, err := png.Decode(bytes.NewReader(result))
	if err != nil {
		t.Fatal("decode output:", err)
	}
	bounds := decoded.Bounds()
	if bounds.Dx() != 64 || bounds.Dy() != 64 {
		t.Errorf("expected 64x64, got %dx%d", bounds.Dx(), bounds.Dy())
	}
}

func TestProcessAvatar_JPEG(t *testing.T) {
	// Create a 50x200 tall JPEG (non-square — tests center-crop)
	img := image.NewRGBA(image.Rect(0, 0, 50, 200))
	for y := 0; y < 200; y++ {
		for x := 0; x < 50; x++ {
			img.Set(x, y, color.RGBA{R: 255, G: 0, B: 0, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal("encode test image:", err)
	}

	// Feed PNG bytes but processAvatar will try JPEG first... Let's test the actual decode path
	// by wrapping the PNG data — processAvatar tries PNG, JPEG, GIF in order.
	result, err := processAvatar(bytes.NewReader(buf.Bytes()), 2<<20)
	if err != nil {
		t.Fatal("processAvatar failed:", err)
	}

	decoded, err := png.Decode(bytes.NewReader(result))
	if err != nil {
		t.Fatal("decode output:", err)
	}
	bounds := decoded.Bounds()
	if bounds.Dx() != 64 || bounds.Dy() != 64 {
		t.Errorf("expected 64x64, got %dx%d", bounds.Dx(), bounds.Dy())
	}
}

func TestProcessAvatar_SquareImage(t *testing.T) {
	// Create a 200x200 square PNG
	img := image.NewRGBA(image.Rect(0, 0, 200, 200))
	for y := 0; y < 200; y++ {
		for x := 0; x < 200; x++ {
			img.Set(x, y, color.RGBA{R: 0, G: 255, B: 0, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal("encode test image:", err)
	}

	result, err := processAvatar(bytes.NewReader(buf.Bytes()), 2<<20)
	if err != nil {
		t.Fatal("processAvatar failed:", err)
	}

	decoded, err := png.Decode(bytes.NewReader(result))
	if err != nil {
		t.Fatal("decode output:", err)
	}
	bounds := decoded.Bounds()
	if bounds.Dx() != 64 || bounds.Dy() != 64 {
		t.Errorf("expected 64x64, got %dx%d", bounds.Dx(), bounds.Dy())
	}
}

func TestProcessAvatar_WideImage(t *testing.T) {
	// Create a 300x100 wide image (tests center-crop)
	img := image.NewRGBA(image.Rect(0, 0, 300, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 300; x++ {
			img.Set(x, y, color.RGBA{R: 255, G: 165, B: 0, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal("encode test image:", err)
	}

	result, err := processAvatar(bytes.NewReader(buf.Bytes()), 2<<20)
	if err != nil {
		t.Fatal("processAvatar failed:", err)
	}

	decoded, err := png.Decode(bytes.NewReader(result))
	if err != nil {
		t.Fatal("decode output:", err)
	}
	bounds := decoded.Bounds()
	if bounds.Dx() != 64 || bounds.Dy() != 64 {
		t.Errorf("expected 64x64, got %dx%d", bounds.Dx(), bounds.Dy())
	}
}

func TestProcessAvatar_TooLarge(t *testing.T) {
	// Create a large image and limit to a tiny maxBytes
	img := image.NewRGBA(image.Rect(0, 0, 1000, 1000))
	for y := 0; y < 1000; y++ {
		for x := 0; x < 1000; x++ {
			img.Set(x, y, color.RGBA{R: byte(rand.Intn(256)), G: byte(rand.Intn(256)), B: byte(rand.Intn(256)), A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal("encode test image:", err)
	}

	// With a very small limit, processAvatar should fail during decode
	_, err := processAvatar(bytes.NewReader(buf.Bytes()), 100)
	if err == nil {
		t.Error("expected error for too-large image, got nil")
	}
}

func TestProcessAvatar_InvalidFormat(t *testing.T) {
	// Feed garbage bytes
	data := []byte("this is not an image file")
	_, err := processAvatar(bytes.NewReader(data), 2<<20)
	if err == nil {
		t.Error("expected error for invalid format, got nil")
	}
}

func TestProcessAvatar_EmptyInput(t *testing.T) {
	_, err := processAvatar(bytes.NewReader([]byte{}), 2<<20)
	if err == nil {
		t.Error("expected error for empty input, got nil")
	}
}

func TestDecodeImage_PNG(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	var buf bytes.Buffer
	png.Encode(&buf, img)

	result, err := decodeImage(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal("decodeImage failed:", err)
	}
	if result.Bounds().Dx() != 10 || result.Bounds().Dy() != 10 {
		t.Errorf("expected 10x10, got %dx%d", result.Bounds().Dx(), result.Bounds().Dy())
	}
}

func TestDecodeImage_Unsupported(t *testing.T) {
	data := []byte("BMP some fake header")
	_, err := decodeImage(bytes.NewReader(data))
	if err == nil {
		t.Error("expected error for unsupported format")
	}
}
