// Copyright 2024 Roxy Light
// SPDX-License-Identifier: ISC

package sqlite

import (
	"testing"
	"unsafe"

	"modernc.org/libc"
	"modernc.org/libc/sys/types"
)

func BenchmarkGoStringN(b *testing.B) {
	tls := libc.NewTLS()
	const want = "Hello, World!\n"
	ptr, err := malloc(tls, types.Size_t(len(want)+1))
	if err != nil {
		b.Fatal(err)
	}
	defer libc.Xfree(tls, ptr)
	for i := 0; i < len(want); i++ {
		*(*byte)(unsafe.Pointer(ptr + uintptr(i))) = want[i]
	}
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		got := goStringN(ptr, len(want))
		if got != want {
			b.Errorf("goStringN(%#x, %d) = %q; want %q", ptr, len(want), got, want)
		}
	}
}
