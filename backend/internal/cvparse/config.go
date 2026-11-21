package cvparse

import (
	"os"
	"os/exec"
	"path/filepath"
)

// ToolsFromEnv membaca lokasi binary dari variabel CV_*; bila kosong dicari di PATH.
//
//	CV_PDFINFO, CV_PDFTOTEXT, CV_PDFTOPPM, CV_PDFIMAGES, CV_TESSERACT, CV_TESSDATA, CV_SOFFICE
func ToolsFromEnv() Tools {
	look := func(env string, names ...string) string {
		if v := os.Getenv(env); v != "" {
			return v
		}
		for _, n := range names {
			if p, err := exec.LookPath(n); err == nil {
				return p
			}
		}
		return ""
	}
	t := Tools{
		PDFInfo:   look("CV_PDFINFO", "pdfinfo"),
		PDFToText: look("CV_PDFTOTEXT", "pdftotext"),
		PDFToPPM:  look("CV_PDFTOPPM", "pdftoppm"),
		PDFImages: look("CV_PDFIMAGES", "pdfimages"),
		Tesseract: look("CV_TESSERACT", "tesseract"),
		Soffice:   look("CV_SOFFICE", "soffice"),
		TessData:  os.Getenv("CV_TESSDATA"),
	}
	if t.TessData == "" && t.Tesseract != "" {
		t.TessData = filepath.Join(filepath.Dir(t.Tesseract), "tessdata")
	}
	return t
}
