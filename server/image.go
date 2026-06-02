package server

import (
	"bytes"
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"log/slog"

	"golang.org/x/image/draw"
)

const AvatarSize = 64

// processAvatar reads an image from r, decodes it, resizes/crops to 64×64
// square with alpha padding for non-square inputs, and returns PNG-encoded bytes.
func processAvatar(r io.Reader, maxBytes int64) ([]byte, error) {
	src := io.LimitReader(r, maxBytes)

	img, err := decodeImage(src)
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}

	bounds := img.Bounds()
	w := bounds.Dx()
	h := bounds.Dy()

	// Crop or pad to square, then resize to AvatarSize
	var square image.Image

	if w == h {
		// Already square — just resize
		square = img
	} else if w > h {
		// Wider than tall — center-crop to square
		offX := (w - h) / 2
		square = &subImage{img, image.Rect(offX, 0, offX+h, h)}
	} else {
		// Taller than wide — center-crop to square
		offY := (h - w) / 2
		square = &subImage{img, image.Rect(0, offY, w, offY+w)}
	}

	// Resize to 64×64 using high-quality interpolation
	dst := image.NewRGBA(image.Rect(0, 0, AvatarSize, AvatarSize))
	draw.ApproxBiLinear.Scale(dst, dst.Bounds(), square, square.Bounds(), draw.Over, nil)

	// Encode as PNG
	var buf bytes.Buffer
	if err := png.Encode(&buf, dst); err != nil {
		return nil, fmt.Errorf("encode png: %w", err)
	}

	slog.Info("Avatar processed",
		"original_size", fmt.Sprintf("%dx%d", w, h),
		"final_size", fmt.Sprintf("%dx%d", AvatarSize, AvatarSize),
		"png_bytes", buf.Len(),
	)

	return buf.Bytes(), nil
}

// decodeImage decodes r as PNG, JPEG, or GIF by peeking at the header.
func decodeImage(r io.Reader) (image.Image, error) {
	header := make([]byte, 4)
	if _, err := io.ReadFull(r, header); err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}

	// Reconstruct reader: header bytes + rest
	rr := io.MultiReader(bytes.NewReader(header), r)

	// PNG magic: 0x89 0x50 0x4E 0x47
	if header[0] == 0x89 && header[1] == 0x50 && header[2] == 0x4E && header[3] == 0x47 {
		img, err := png.Decode(rr)
		if err != nil {
			return nil, fmt.Errorf("decode png: %w", err)
		}
		return img, nil
	}

	// JPEG magic: 0xFF 0xD8
	if header[0] == 0xFF && header[1] == 0xD8 {
		img, err := jpeg.Decode(rr)
		if err != nil {
			return nil, fmt.Errorf("decode jpeg: %w", err)
		}
		return img, nil
	}

	// GIF magic: "GIF"
	if header[0] == 'G' && header[1] == 'I' && header[2] == 'F' {
		img, err := gif.Decode(rr)
		if err != nil {
			return nil, fmt.Errorf("decode gif: %w", err)
		}
		return img, nil
	}

	return nil, fmt.Errorf("unsupported format (supported: PNG, JPEG, GIF)")
}

// subImage wraps an image with a sub-bounds for cropping without copying pixels.
type subImage struct {
	image.Image
	rect image.Rectangle
}

func (s *subImage) Bounds() image.Rectangle { return s.rect }
