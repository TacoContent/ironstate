package filters

// Pool keeps track of every scriptWorker started so far, keyed by filter
// name, and closes them all together - one persistent subprocess per
// discovered script filter for the process's lifetime.

import "sync"

type Pool struct {
	mu      sync.Mutex
	workers map[string]*scriptWorker
}

// NewPool returns an empty Pool.
func NewPool() *Pool {
	return &Pool{workers: map[string]*scriptWorker{}}
}

// WorkerFor returns the persistent worker for name, creating (but not yet
// starting - see scriptWorker.ensureStarted) one on first use.
func (p *Pool) WorkerFor(name string, argv []string) *scriptWorker {
	p.mu.Lock()
	defer p.mu.Unlock()
	if w, ok := p.workers[name]; ok {
		return w
	}
	w := newScriptWorker(argv)
	p.workers[name] = w
	return w
}

// Close terminates every worker's subprocess (if started). Returns the
// first error encountered, if any, after attempting to close them all.
func (p *Pool) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	var firstErr error
	for _, w := range p.workers {
		if err := w.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
