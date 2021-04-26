package sshd

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

type fakeNewChannel struct {
	channelType string
	extraData   []byte
}

func (f *fakeNewChannel) Accept() (ssh.Channel, <-chan *ssh.Request, error) {
	return nil, nil, nil
}

func (f *fakeNewChannel) Reject(reason ssh.RejectionReason, message string) error {
	return nil
}

func (f *fakeNewChannel) ChannelType() string {
	return f.channelType
}

func (f *fakeNewChannel) ExtraData() []byte {
	return f.extraData
}

func TestPanicDuringSessionIsRecovered(t *testing.T) {
	numSessions := 0
	conn := newConnection(1, "127.0.0.1:50000")

	newChannel := &fakeNewChannel{channelType: "session"}
	chans := make(chan ssh.NewChannel, 1)
	chans <- newChannel

	require.NotPanics(t, func() {
		conn.handle(context.Background(), chans, func(context.Context, ssh.Channel, <-chan *ssh.Request) {
			numSessions += 1
			close(chans)
			panic("This is a panic")
		})
	})

	require.Equal(t, numSessions, 1)
}
