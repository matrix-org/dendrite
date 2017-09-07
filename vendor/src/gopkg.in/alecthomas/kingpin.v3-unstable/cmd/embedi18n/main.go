package main

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
)

func compress(data []byte) []byte {
	w := bytes.NewBuffer(nil)
	gw, err := gzip.NewWriterLevel(w, gzip.BestCompression)
	if err != nil {
		panic(err)
	}
	_, err = gw.Write(data)
	if err != nil {
		panic(err)
	}
	gw.Close()
	return w.Bytes()
}

func main() {
	name := os.Args[1]
	r, err := os.Open("i18n/" + name + ".all.json")
	if err != nil {
		panic(err)
	}
	defer r.Close()
	data, err := ioutil.ReadAll(r)
	if err != nil {
		panic(err)
	}
	data = compress(data)
	id := strings.Replace(name, "-", "_", -1)
	w, err := os.Create("i18n_" + id + ".go")
	if err != nil {
		panic(err)
	}
	defer w.Close()
	fmt.Fprintf(w, `package kingpin

var i18n_%s = []byte(%q)
`, id, data)
}
