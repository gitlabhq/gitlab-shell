package reporting

import "io"

type Reporter struct {
	Out    io.Writer
	ErrOut io.Writer
}
