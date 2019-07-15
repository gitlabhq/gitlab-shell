package executable

import (
	"fmt"
	"os"
	"path/filepath"
)

type Executable struct {
	Name string
}

var (
	// osExecutable is overridden in tests
	osExecutable = os.Executable
)

func New() (*Executable, error) {
	path, err := osExecutable()
	if err != nil {
		return nil, err
	}

	return &Executable{Name: filepath.Base(path)}, nil
}

func (e *Executable) IsForExecutingCommand() bool {
	return e.Name == "gitlab-shell"
}

func (e *Executable) FallbackProgram() string {
	return fmt.Sprintf("%s-ruby", e.Name)
}
