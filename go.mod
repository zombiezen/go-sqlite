module zombiezen.com/go/sqlite

go 1.23.0

toolchain go1.24.2

require (
	crawshaw.io/iox v0.0.0-20181124134642-c51c3df30797
	github.com/chzyer/readline v1.5.0
	github.com/google/go-cmp v0.6.0
	golang.org/x/text v0.14.0
	modernc.org/libc v1.65.7
	modernc.org/sqlite v1.37.1
)

require (
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/ncruces/go-strftime v0.1.9 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	golang.org/x/exp v0.0.0-20250408133849-7e4ce0ab07d0 // indirect
	golang.org/x/sys v0.33.0 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.11.0 // indirect
)

retract (
	v0.9.1 // Contains retractions only.
	v0.9.0 // Had libc memgrind issues.
)
