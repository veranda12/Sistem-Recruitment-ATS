package cvparse

import (
	"context"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf16"
)

var cfbMagic = []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}

// cfbInfo memvalidasi Compound File Binary dan mencari stream yang menentukan jenis dokumen.
type cfbInfo struct {
	streams map[string]bool
	word    []byte // 512 byte pertama stream WordDocument (FIB)
}

// parseCFB membaca header, FAT (termasuk DIFAT lanjutan), dan direktori. Tidak menjalankan apa pun.
func parseCFB(b []byte) (*cfbInfo, error) {
	if len(b) < 512 || string(b[:8]) != string(cfbMagic) {
		return nil, fail(ReasonUnsupported, "bukan CFB")
	}
	le := binary.LittleEndian
	shift := le.Uint16(b[30:])
	if shift != 9 && shift != 12 {
		return nil, fail(ReasonCorrupt, "ukuran sektor")
	}
	ss := 1 << shift
	sector := func(n uint32) ([]byte, bool) {
		off := (int64(n) + 1) * int64(ss)
		if n >= 0xFFFFFFFA || off < 0 || off+int64(ss) > int64(len(b)) {
			return nil, false
		}
		return b[off : off+int64(ss)], true
	}
	// DIFAT: 109 entri di header, lanjutan berantai.
	var fatSecs []uint32
	for i := 0; i < 109; i++ {
		v := le.Uint32(b[76+i*4:])
		if v < 0xFFFFFFFA {
			fatSecs = append(fatSecs, v)
		}
	}
	next := le.Uint32(b[68:])
	for guard := 0; next < 0xFFFFFFFA && guard < 10000; guard++ {
		s, ok := sector(next)
		if !ok {
			return nil, fail(ReasonCorrupt, "DIFAT")
		}
		n := ss/4 - 1
		for i := 0; i < n; i++ {
			if v := le.Uint32(s[i*4:]); v < 0xFFFFFFFA {
				fatSecs = append(fatSecs, v)
			}
		}
		next = le.Uint32(s[n*4:])
	}
	var fat []uint32
	for _, fs := range fatSecs {
		s, ok := sector(fs)
		if !ok {
			return nil, fail(ReasonCorrupt, "FAT")
		}
		for i := 0; i < ss/4; i++ {
			fat = append(fat, le.Uint32(s[i*4:]))
		}
	}
	chain := func(start uint32, limit int) ([]byte, error) {
		var out []byte
		for cur, guard := start, 0; cur < 0xFFFFFFFA; guard++ {
			if guard > len(fat) || int(cur) >= len(fat) {
				return nil, fail(ReasonCorrupt, "rantai sektor")
			}
			s, ok := sector(cur)
			if !ok {
				return nil, fail(ReasonCorrupt, "sektor")
			}
			out = append(out, s...)
			if limit > 0 && len(out) >= limit {
				return out[:limit], nil
			}
			cur = fat[cur]
		}
		return out, nil
	}
	dir, err := chain(le.Uint32(b[48:]), 0)
	if err != nil {
		return nil, err
	}
	info := &cfbInfo{streams: map[string]bool{}}
	miniCutoff := le.Uint32(b[56:])
	for off := 0; off+128 <= len(dir); off += 128 {
		e := dir[off : off+128]
		nlen := int(le.Uint16(e[64:]))
		if nlen < 2 || nlen > 64 {
			continue
		}
		u := make([]uint16, nlen/2-1)
		for i := range u {
			u[i] = le.Uint16(e[i*2:])
		}
		name := string(utf16.Decode(u))
		typ := e[66]
		info.streams[name] = true
		if name == "WordDocument" && typ == 2 {
			size := le.Uint64(e[120:])
			if size < 32 {
				return nil, fail(ReasonCorrupt, "FIB")
			}
			if uint32(size) < miniCutoff {
				return nil, fail(ReasonCorrupt, "WordDocument terlalu kecil")
			}
			fib, err := chain(le.Uint32(e[116:]), 512)
			if err != nil {
				return nil, err
			}
			info.word = fib
		}
	}
	return info, nil
}

// validateDoc: Word 97-2003 asli memiliki stream WordDocument dengan FIB magic 0xA5EC.
func validateDoc(b []byte) error {
	info, err := parseCFB(b)
	if err != nil {
		return err
	}
	if info.word == nil {
		return fail(ReasonUnsupported, "CFB tanpa WordDocument (bukan .doc)")
	}
	le := binary.LittleEndian
	if le.Uint16(info.word[0:]) != 0xA5EC {
		return fail(ReasonCorrupt, "FIB magic")
	}
	if le.Uint16(info.word[0x0A:])&0x0100 != 0 {
		return fail(ReasonEncrypted, "doc terenkripsi")
	}
	for name := range info.streams {
		n := strings.ToLower(name)
		if n == "macros" || n == "_vba_project_cur" || n == "vba" {
			return fail(ReasonMacro, "doc bermakro")
		}
	}
	return nil
}

// sofficeProfile membuat profil LibreOffice sementara: makro dimatikan, tanpa pemeriksaan pembaruan,
// tanpa memuat tautan eksternal.
const sofficeRegistry = `<?xml version="1.0" encoding="UTF-8"?>
<oor:items xmlns:oor="http://openoffice.org/2001/registry" xmlns:xs="http://www.w3.org/2001/XMLSchema" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
<item oor:path="/org.openoffice.Office.Common/Security/Scripting"><prop oor:name="MacroSecurityLevel" oor:op="fuse"><value>3</value></prop></item>
<item oor:path="/org.openoffice.Office.Common/Security/Scripting"><prop oor:name="DisableMacrosExecution" oor:op="fuse"><value>true</value></prop></item>
<item oor:path="/org.openoffice.Office.Common/Misc"><prop oor:name="FirstRun" oor:op="fuse"><value>false</value></prop></item>
<item oor:path="/org.openoffice.Office.Common/Load"><prop oor:name="UpdateDocMode" oor:op="fuse"><value>0</value></prop></item>
<item oor:path="/org.openoffice.Office.Calc/Formula/Load"><prop oor:name="OOXMLRecalcMode" oor:op="fuse"><value>1</value></prop></item>
<item oor:path="/org.openoffice.Office.Common/Security"><prop oor:name="BlockUntrustedRefererLinks" oor:op="fuse"><value>true</value></prop></item>
</oor:items>`

// convertToPDF mengonversi DOC/DOCX ke PDF privat lewat LibreOffice headless dengan profil sementara per job.
// Nama berkas pengguna tidak dipakai sebagai path.
func (e *engine) convertToPDF(ctx context.Context, data []byte, ext string) (string, error) {
	if e.tools.Soffice == "" {
		return "", fail(ReasonToolMissing, "konverter dokumen tidak dikonfigurasi")
	}
	in := filepath.Join(e.dir, "input"+ext)
	if err := os.WriteFile(in, data, 0o400); err != nil {
		return "", err
	}
	profile := filepath.Join(e.dir, "lo-profile")
	if err := os.MkdirAll(filepath.Join(profile, "user"), 0o700); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(profile, "user", "registrymodifications.xcu"), []byte(sofficeRegistry), 0o600); err != nil {
		return "", err
	}
	outDir := filepath.Join(e.dir, "converted")
	if err := os.MkdirAll(outDir, 0o700); err != nil {
		return "", err
	}
	_, err := run(ctx, e.lim.StageConvert, e.dir, e.env(), 1<<20, e.tools.Soffice,
		"-env:UserInstallation="+fileURL(profile), "--headless", "--invisible", "--norestore", "--nolockcheck", "--nodefault",
		"--nologo", "--nofirststartwizard", "--convert-to", "pdf", "--outdir", outDir, in)
	if err != nil {
		var te *toolError
		if errors.As(err, &te) {
			return "", transient(ReasonToolCrashed, "konversi gagal")
		}
		return "", err
	}
	out := filepath.Join(outDir, "input.pdf")
	if st, err := os.Stat(out); err != nil || st.Size() == 0 {
		return "", fail(ReasonCorrupt, "konverter tidak menghasilkan PDF")
	}
	return out, nil
}

func fileURL(p string) string {
	p = filepath.ToSlash(p)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return "file://" + p
}
