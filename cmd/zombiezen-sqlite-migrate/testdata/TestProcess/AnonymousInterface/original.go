// Copyright 2021 Ross Light
// SPDX-License-Identifier: ISC

package main

func main() {
	var foo interface {
		Bar()
	}
	_ = foo
}
