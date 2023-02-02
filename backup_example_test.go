package sqlite_test

import (
	"fmt"
	"os"
	"time"

	"zombiezen.com/go/sqlite"
)

// This example shows the basic use of a backup object.
func ExampleBackup() {
	// Open database connections.
	src, err := sqlite.OpenConn(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer src.Close()
	dst, err := sqlite.OpenConn(os.Args[2])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer func() {
		if err := dst.Close(); err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
	}()

	// Create Backup object.
	backup, err := sqlite.NewBackup(dst, "main", src, "main")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	// Perform online backup/copy.
	_, err1 := backup.Step(-1)
	err2 := backup.Close()
	if err1 != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err2 != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// This example shows how to use Step
// to prevent holding a read lock on the source database
// during the entire copy.
func ExampleBackup_Step() {
	// Open database connections.
	src, err := sqlite.OpenConn(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer src.Close()
	dst, err := sqlite.OpenConn(os.Args[2])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer func() {
		if err := dst.Close(); err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
	}()

	// Create Backup object.
	backup, err := sqlite.NewBackup(dst, "main", src, "main")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer func() {
		if err := backup.Close(); err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
	}()

	// Each iteration of this loop copies 5 database pages,
	// waiting 250ms between iterations.
	for {
		more, err := backup.Step(5)
		if !more {
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
}
