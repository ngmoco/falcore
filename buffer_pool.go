package falcore

import (
	"bufio"
	"io"
	"io/ioutil"
)

// uses a chan as a leaky bucket buffer pool
type bufferPool struct {
	// size of buffer when creating new ones
	bufSize int
	// the actual pool of buffers ready for reuse
	pool chan *bufferPoolEntry
}

// This is what's stored in the buffer.  It allows
// for the underlying io.Reader to be changed out
// inside a bufio.Reader.  This is required for reuse.
type bufferPoolEntry struct {
	br     *bufio.Reader
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
	}
}

// Take a buffer from the pool and set 
// it up to read from r
func (p *bufferPool) take(r io.Reader) (bpe *bufferPoolEntry) {
	select {
	case bpe = <-p.pool:
		// prepare for reuse
		if a := bpe.br.Buffered(); a > 0 {
			// drain the internal buffer
			io.CopyN(ioutil.Discard, bpe.br, int64(a))
		}
		// swap out the underlying reader
		bpe.source = r
	default:
		// none available.  create a new one
		bpe = &bufferPoolEntry{nil, r}
		bpe.br = bufio.NewReaderSize(bpe, p.bufSize)
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
