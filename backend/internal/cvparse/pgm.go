package cvparse

import (
	"bufio"
	"errors"
	"image"
	"image/color"
	"io"
	"strconv"
)

// Decoder PGM biner (P5, 8-bit) untuk render OCR dari pdftoppm -gray.
func init() {
	image.RegisterFormat("pgm", "P5", decodePGM, decodePGMConfig)
}

func pgmHeader(r *bufio.Reader) (w, h, maxv int, err error) {
	var fields []int
	magic := make([]byte, 2)
	if _, err = io.ReadFull(r, magic); err != nil || string(magic) != "P5" {
		return 0, 0, 0, errors.New("bukan PGM P5")
	}
	for len(fields) < 3 {
		b, err := r.ReadByte()
		if err != nil {
			return 0, 0, 0, err
		}
		switch {
		case b == '#':
			if _, err := r.ReadString('\n'); err != nil {
				return 0, 0, 0, err
			}
		case b >= '0' && b <= '9':
			num := []byte{b}
			for {
				c, err := r.ReadByte()
				if err != nil {
					return 0, 0, 0, err
				}
				if c < '0' || c > '9' {
					break
				}
				num = append(num, c)
			}
			v, err := strconv.Atoi(string(num))
			if err != nil || v <= 0 || v > 1<<20 {
				return 0, 0, 0, errors.New("header PGM tidak valid")
			}
			fields = append(fields, v)
		}
	}
	if fields[2] > 255 {
		return 0, 0, 0, errors.New("PGM 16-bit tidak didukung")
	}
	return fields[0], fields[1], fields[2], nil
}

func decodePGMConfig(r io.Reader) (image.Config, error) {
	w, h, _, err := pgmHeader(bufio.NewReader(r))
	if err != nil {
		return image.Config{}, err
	}
	return image.Config{ColorModel: color.GrayModel, Width: w, Height: h}, nil
}

func decodePGM(r io.Reader) (image.Image, error) {
	br := bufio.NewReader(r)
	w, h, _, err := pgmHeader(br)
	if err != nil {
		return nil, err
	}
	g := image.NewGray(image.Rect(0, 0, w, h))
	if _, err := io.ReadFull(br, g.Pix); err != nil {
		return nil, err
	}
	return g, nil
}
