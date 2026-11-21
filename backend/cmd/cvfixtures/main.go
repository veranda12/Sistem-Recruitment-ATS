// cvfixtures membuat fixture CV sintetis + golden JSON yang dapat direproduksi.
//
//	go run ./cmd/cvfixtures -out ../tests/cv-fixtures/out
package main

import (
	"flag"
	"fmt"
	"os"

	"ats/internal/cvfixture"
)

func main() {
	out := flag.String("out", "../tests/cv-fixtures/out", "direktori keluaran")
	flag.Parse()
	t := cvfixture.Tools{PDFToPPM: os.Getenv("CV_PDFTOPPM"), Soffice: os.Getenv("CV_SOFFICE"), Python: os.Getenv("CV_PYTHON")}
	if err := os.RemoveAll(*out); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	list, err := cvfixture.Generate(*out, t, func(f string, a ...any) { fmt.Fprintf(os.Stderr, "peringatan: "+f+"\n", a...) })
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	per := map[string]int{}
	for _, f := range list {
		per[f.Split+"/"+f.Format]++
	}
	fmt.Printf("%d fixture ditulis ke %s\n", len(list), *out)
	for k, v := range per {
		fmt.Printf("  %-24s %d\n", k, v)
	}
}
