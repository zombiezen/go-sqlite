module zombiezen.com/go/sqlite

go 1.16

require (
	crawshaw.io/iox v0.0.0-20181124134642-c51c3df30797
	github.com/chzyer/readline v1.5.0
	github.com/google/go-cmp v0.5.9
	modernc.org/libc v1.21.5
	modernc.org/sqlite v1.20.0
)

retract (
	v0.9.1 // Contains retractions only.
	v0.9.0 // Had libc memgrind issues.
)
