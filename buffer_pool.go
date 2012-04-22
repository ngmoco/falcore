package falcore

import (
	"bufio"
	"io"
)

// uses a chan as a leaky bucket buffer pool
type bufferPool struct {
	// size of buffer when creating new ones
	bufSize int
	// the actual pool of buffers ready for reuse
	pool chan *bufferPoolEntry
	// this is used for draining a buffer to prep for reuse
	drain []byte
}

// This is what's stored in the buffer.  It allows
// for the underlying io.Reader to be changed out
// inside a bufio.Reader.  This is required for reuse.
type bufferPoolEntry struct {
	buf    *bufio.Reader
	source io.Reader
}

// make bufferPoolEntry a passthrough io.Reader
func (bpe *bufferPoolEntry) Read(p []byte) (n int, err error) {
	return bpe.source.Read(p)
}

func newBufferPool(poolSize, bufferSize int) *bufferPool {
	return &bufferPool{
		bufSize: bufferSize,
		pool:    make(chan *bufferPoolEntry, poolSize),
		drain:   make([]byte, bufferSize),
	}
}

// Take a buffer from the pool and set 
// it up to read from r
func (p *bufferPool) take(r io.Reader) (bpe *bufferPoolEntry) {
	select {
	case bpe = <-p.pool:
		// prepare for reuse
		if a := bpe.buf.Buffered(); a > 0 {
			// drain the internal buffer
			bpe.buf.Read(p.drain[0:a])
		}
		// swap out the underlying reader
		bpe.source = r
	default:
		// none available.  create a new one
		bpe = &bufferPoolEntry{nil, r}
		bpe.buf = bufio.NewReaderSize(bpe, p.bufSize)
	}
	return
}

// Return a buffer to the pool
func (p *bufferPool) give(bpe *bufferPoolEntry) {
	select {
	case p.pool <- bpe: // return to pool
	default: // discard
	}
}
