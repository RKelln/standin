// gobench measures plain Go float32 matmul throughput, to price a hand-written
// transformer forward pass (no cgo, no SIMD kernels).
//
//	go run gobench.go
package main

import (
	"fmt"
	"time"
)

func main() {
	const N = 512
	a := make([]float32, N*N)
	b := make([]float32, N*N)
	c := make([]float32, N*N)
	for i := range a {
		a[i] = 0.5
		b[i] = 0.25
	}
	t0 := time.Now()
	for i := 0; i < N; i++ {
		for k := 0; k < N; k++ {
			av := a[i*N+k]
			bk := b[k*N : k*N+N]
			ci := c[i*N : i*N+N]
			for j := 0; j < N; j++ {
				ci[j] += av * bk[j]
			}
		}
	}
	d := time.Since(t0)
	flops := 2.0 * N * N * N
	fmt.Printf("scalar Go matmul %dx%dx%d: %v -> %.2f GFLOPS\n", N, N, N, d, flops/d.Seconds()/1e9)
}
