module zombiezen.com/go/sqlite

go 1.19

require (
	crawshaw.io/iox v0.0.0-20181124134642-c51c3df30797
	github.com/chzyer/readline v1.5.0
	github.com/google/go-cmp v0.5.9
	modernc.org/libc v1.22.2
	modernc.org/sqlite v1.20.5-0.20230220170856-13895386cf24
)

require (
	github.com/dustin/go-humanize v1.0.0 // indirect
	github.com/google/uuid v1.3.0 // indirect
	github.com/mattn/go-isatty v0.0.16 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20200410134404-eec4a21b6bb0 // indirect
	golang.org/x/sys v0.0.0-20220811171246-fbc7d0a398ab // indirect
	modernc.org/mathutil v1.5.0 // indirect
	modernc.org/memory v1.4.0 // indirect
)

retract (
	v0.9.1 // Contains retractions only.
	v0.9.0 // Had libc memgrind issues.
)
