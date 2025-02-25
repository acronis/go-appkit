/*
Copyright © 2025 Acronis International GmbH.

Released under MIT license.
*/

package lrucache

import (
	"bytes"
	"errors"
	"fmt"
	"runtime/debug"
	"sync"
)

type singleFlightCall[V any] struct {
	wg  sync.WaitGroup
	val V
	err error
}

type singleFlightGroup[K comparable, V any] struct {
	mu sync.Mutex
	m  map[K]*singleFlightCall[V]
}

func (g *singleFlightGroup[K, V]) Do(key K, fn func() (V, error)) (V, error) {
	g.mu.Lock()
	if g.m == nil {
		g.m = make(map[K]*singleFlightCall[V])
	}
	if c, ok := g.m[key]; ok {
		g.mu.Unlock()
		c.wg.Wait()
		return c.val, c.err
	}
	c := &singleFlightCall[V]{}
	c.wg.Add(1)
	g.m[key] = c
	g.mu.Unlock()

	return g.do(c, key, fn)
}

func (g *singleFlightGroup[K, V]) do(c *singleFlightCall[V], key K, fn func() (V, error)) (val V, err error) {
	normalReturn := false
	recovered := false

	// double-defer to distinguish panic from runtime.Goexit
	defer func() {
		if !normalReturn && !recovered {
			c.err = ErrGoexit
		}

		c.wg.Done()

		g.mu.Lock()
		delete(g.m, key)
		g.mu.Unlock()

		if recovered {
			panic(c.err.(*PanicError).Value) // re-panic on the same goroutine
		}

		val, err = c.val, c.err
	}()

	defer func() {
		if !normalReturn {
			if v := recover(); v != nil {
				c.err = newPanicError(v)
				recovered = true
			}
		}
	}()
	c.val, c.err = fn()
	normalReturn = true

	return c.val, c.err // will be set in the defer
}

// ErrGoexit is returned when a goroutine calls runtime.Goexit.
var ErrGoexit = errors.New("runtime.Goexit was called")

// PanicError is an error that represents a panic value and stack trace.
type PanicError struct {
	Value interface{}
	Stack []byte
}

func (p *PanicError) Error() string {
	return fmt.Sprintf("%v\n\n%s", p.Value, p.Stack)
}

func newPanicError(v interface{}) error {
	stack := debug.Stack()

	// The first line of the stack trace is of the form "goroutine N [status]:"
	// but by the time the panic reaches Do the goroutine may no longer exist
	// and its status will have changed. Trim out the misleading line.
	if line := bytes.IndexByte(stack, '\n'); line >= 0 {
		stack = stack[line+1:]
	}
	return &PanicError{Value: v, Stack: stack}
}

func (p *PanicError) Unwrap() error {
	err, ok := p.Value.(error)
	if !ok {
		return nil
	}
	return err
}
