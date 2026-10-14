package runner

import (
	"bytes"
	"io"
	"testing"
)

func TestErrorColorOnlyInConsole(t *testing.T) {
	var console, file bytes.Buffer
	w := io.MultiWriter(errorConsoleWriter{&console}, &file)
	message := []byte("[ERROR] SIUI OCSP HTTP=471\n")
	if n, err := w.Write(message); err != nil || n != len(message) {
		t.Fatal(n, err)
	}
	if !bytes.Contains(console.Bytes(), []byte("\x1b[31m")) || !bytes.Equal(file.Bytes(), message) {
		t.Fatal("color routing")
	}
	console.Reset()
	file.Reset()
	w.Write([]byte("token obtinut\n"))
	if !bytes.Equal(console.Bytes(), file.Bytes()) {
		t.Fatal("success colored as error")
	}
}
