package model

import "sync"

const chunkSize = 5

type settled[T any] struct {
	value T
	err   error
}

func forChunks(n int, fn func(from, to int)) {
	if n == 0 {
		return
	}
	for from := 0; from < n; from += chunkSize {
		to := from + chunkSize
		if to > n {
			to = n
		}
		fn(from, to)
	}
}

func runSettled[T any](n int, fn func(i int) (T, error)) []settled[T] {
	out := make([]settled[T], n)
	if n == 0 {
		return out
	}
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		i := i
		go func() {
			defer wg.Done()
			v, err := fn(i)
			out[i] = settled[T]{value: v, err: err}
		}()
	}
	wg.Wait()
	return out
}
