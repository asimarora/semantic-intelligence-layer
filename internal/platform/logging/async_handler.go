package logging

import (
        "context"
        "log/slog"
        "sync"
        "time"
)

type asyncHandler struct {
        state   *asyncState
        handler slog.Handler
}

type asyncState struct {
        mu      sync.Mutex
        once    sync.Once
        queue   chan asyncEntry
        done    chan struct{}
        closed  bool
        base    slog.Handler
        dropped uint64
}

type asyncEntry struct {
        ctx     context.Context
        handler slog.Handler
        record  slog.Record
}

func newAsyncHandler(handler slog.Handler, queueSize int) *asyncHandler {
        state := &asyncState{
                queue: make(chan asyncEntry, queueSize),
                done:  make(chan struct{}),
                base:  handler,
        }
        go state.run()

        return &asyncHandler{
                state:   state,
                handler: handler,
        }
}

func (h *asyncHandler) Enabled(ctx context.Context, level slog.Level) bool {
        return h.handler.Enabled(ctx, level)
}

func (h *asyncHandler) Handle(ctx context.Context, record slog.Record) error {
        record = record.Clone()

        h.state.mu.Lock()
        if h.state.closed {
                h.state.mu.Unlock()
                return h.handler.Handle(ctx, record)
        }

        select {
        case h.state.queue <- asyncEntry{ctx: ctx, handler: h.handler, record: record}:
                h.state.mu.Unlock()
                return nil
        default:
                h.state.dropped++
                h.state.mu.Unlock()
                if record.Level >= slog.LevelError {
                        return h.handler.Handle(ctx, record)
                }
                return nil
        }
}

func (h *asyncHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
        return &asyncHandler{
                state:   h.state,
                handler: h.handler.WithAttrs(attrs),
        }
}

func (h *asyncHandler) WithGroup(name string) slog.Handler {
        return &asyncHandler{
                state:   h.state,
                handler: h.handler.WithGroup(name),
        }
}

func (h *asyncHandler) Shutdown(ctx context.Context) error {
        h.state.once.Do(func() {
                h.state.mu.Lock()
                h.state.closed = true
                close(h.state.queue)
                h.state.mu.Unlock()
        })

        select {
        case <-h.state.done:
                return nil
        case <-ctx.Done():
                return ctx.Err()
        }
}

func (s *asyncState) run() {
        for entry := range s.queue {
                s.flushDropped(entry.ctx, entry.handler)
                _ = entry.handler.Handle(entry.ctx, entry.record)
        }
        s.flushDropped(context.Background(), s.base)
        close(s.done)
}

func (s *asyncState) flushDropped(ctx context.Context, handler slog.Handler) {
        s.mu.Lock()
        dropped := s.dropped
        s.dropped = 0
        s.mu.Unlock()

        if dropped == 0 {
                return
        }

        record := slog.NewRecord(time.Now(), slog.LevelWarn, "dropped log records", 0)
        record.AddAttrs(slog.Uint64("dropped_count", dropped))
        _ = handler.Handle(ctx, record)
}
