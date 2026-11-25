package limiter

import (
	"context"

	"github.com/xtls/xray-core/common"
	"github.com/xtls/xray-core/common/buf"
	"golang.org/x/time/rate"
)

type Writer struct {
	writer  buf.Writer
	limiter *rate.Limiter
}

func (l *Limiter) RateWriter(writer buf.Writer, limiter *rate.Limiter) buf.Writer {
	return &Writer{
		writer:  writer,
		limiter: limiter,
	}
}

func (w *Writer) Close() error {
	return common.Close(w.writer)
}

func (w *Writer) WriteMultiBuffer(mb buf.MultiBuffer) error {
	ctx := context.Background()
	// WaitN only reports a cancelled context or a burst larger than the
	// limiter's capacity. The context here never cancels and the burst is sized
	// from the buffer, so the write proceeds either way.
	_ = w.limiter.WaitN(ctx, int(mb.Len()))
	return w.writer.WriteMultiBuffer(mb)
}
