package logger

import (
  "fmt"
  "github.com/sirupsen/logrus"
  "github.com/sirupsen/logrus/hooks/test"
  "github.com/stretchr/testify/assert"
  "testing"
)

func TestSomething(t *testing.T){
  logger, hook := test.NewNullLogger()

  logger.Error("Helloerror")

  assert.Equal(t, 1, len(hook.Entries))
  assert.Equal(t, logrus.ErrorLevel, hook.LastEntry().Level)
  assert.Equal(t, "Helloerror", hook.LastEntry().Message)

  // TODO Check timestamp format here
  assert.Equal(t, "", hook.LastEntry().Time)

  fmt.Println(hook.LastEntry().Time)

  hook.Reset()
  assert.Nil(t, hook.LastEntry())
}
