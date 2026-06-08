package voice

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
)

// multipartWriter is a minimal helper that mimics the parts of mime/multipart.Writer
// we need. The standard library is fine, but we rewrap here so call sites stay
// terse.
type mw struct {
	w   *multipart.Writer
	buf *bytes.Buffer
}

func multipartWriter(buf *bytes.Buffer) *mw {
	return &mw{w: multipart.NewWriter(buf), buf: buf}
}

func (m *mw) WriteField(name, value string) error {
	return m.w.WriteField(name, value)
}

func (m *mw) WriteFile(name, filename string, content []byte) error {
	fw, err := m.w.CreateFormFile(name, filename)
	if err != nil {
		return err
	}
	_, err = fw.Write(content)
	return err
}

func (m *mw) Close() error { return m.w.Close() }

func (m *mw) ContentType() string { return m.w.FormDataContentType() }

// readAll for io in this package (avoids the io import in voice.go shadow).
func readAll(r io.Reader) ([]byte, error) {
	return io.ReadAll(r)
}

var _ = fmt.Sprintf // keep fmt imported even if unused
