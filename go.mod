module zombiezen.com/go/sqlite

go 1.20

require (
	crawshaw.io/iox v0.0.0-20181124134642-c51c3df30797
	github.com/chzyer/readline v1.5.0
	github.com/google/go-cmp v0.5.9
	golang.org/x/text v0.14.0
	modernc.org/libc v1.55.3
	modernc.org/sqlite v1.33.1
)

require (
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/ncruces/go-strftime v0.1.9 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	golang.org/x/sys v0.22.0 // indirect
	modernc.org/mathutil v1.6.0 // indirect
	modernc.org/memory v1.8.0 // indirect
)

retract (
	v0.9.1 // Contains retractions only.
	v0.9.0 // Had libc memgrind issues.
)
