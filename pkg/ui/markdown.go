package ui

import (
	"bytes"
	"context"
	"io"
	"sync"

	"github.com/a-h/templ"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
)

// md renders GitHub-flavored markdown. Raw HTML in the source is escaped
// (goldmark's default) — that is the sanitization story; never enable
// html.WithUnsafe here.
var md = goldmark.New(goldmark.WithExtensions(extension.GFM))

var mdPool = sync.Pool{New: func() any { return new(bytes.Buffer) }}

// Markdown renders user- or LLM-authored markdown (entries, summaries,
// notes) as styled HTML. Typography comes from the design system's `prose`
// styles (design/input.css).
func Markdown(src string) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		buf := mdPool.Get().(*bytes.Buffer)
		defer mdPool.Put(buf)
		buf.Reset()
		if err := md.Convert([]byte(src), buf); err != nil {
			return err
		}
		if _, err := io.WriteString(w, `<div class="prose prose-sm dark:prose-invert max-w-none">`); err != nil {
			return err
		}
		if _, err := w.Write(buf.Bytes()); err != nil {
			return err
		}
		_, err := io.WriteString(w, `</div>`)
		return err
	})
}
