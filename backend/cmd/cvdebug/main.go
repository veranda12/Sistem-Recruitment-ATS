// cvdebug mencetak keputusan halaman, urutan baca, dan saran untuk satu berkas (alat pengembang;
// jangan dipakai pada CV nyata karena mencetak isi dokumen).
//
//	go run ./cmd/cvdebug <berkas>
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"ats/internal/cvparse"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "pemakaian: cvdebug <berkas>")
		os.Exit(2)
	}
	work, _ := os.MkdirTemp("", "cvdebug-")
	defer os.RemoveAll(work)
	res, err := cvparse.Process(context.Background(), cvparse.ToolsFromEnv(), cvparse.DefaultLimits(), os.Args[1], work, work+"/out", nil)
	if err != nil {
		fmt.Println("ERROR:", err)
	}
	if res == nil {
		return
	}
	m, _ := json.MarshalIndent(res.Manifest.PageDecisions, "", " ")
	fmt.Println(string(m))
	for _, l := range cvparse.DebugOrder(res.Doc) {
		fmt.Println(l)
	}
	for _, s := range cvparse.Extract(res.Doc).Suggestions {
		fmt.Printf("%-28s %-40q %v %v %v\n", s.Path, s.Raw, s.Evidence, s.ReadDoubt, s.AssocDoubt)
	}
}
