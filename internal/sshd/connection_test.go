package sshd

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

type rejectCall struct {
	reason ssh.RejectionReason
	message string
}

type fakeNewChannel struct {
	channelType string
	extraData   []byte
	rejectCh chan rejectCall
}

func (f *fakeNewChannel) Accept() (ssh.Channel, <-chan *ssh.Request, error) {
	return nil, nil, nil
}

func (f *fakeNewChannel) Reject(reason ssh.RejectionReason, message string) error {
	f.rejectCh <- rejectCall{reason: reason, message: message}

	return nil
}

func (f *fakeNewChannel) ChannelType() string {
	return f.channelType
}

func (f *fakeNewChannel) ExtraData() []byte {
	return f.extraData
}

func setup(sessionsNum int64, newChannel *fakeNewChannel) (*connection, chan ssh.NewChannel) {
	conn := newConnection(sessionsNum, "127.0.0.1:50000")

	chans := make(chan ssh.NewChannel, 1)
	chans <- newChannel

	return conn, chans
}

func TestPanicDuringSessionIsRecovered(t *testing.T) {
	newChannel := &fakeNewChannel{channelType: "session"}
	conn, chans := setup(1, newChannel)

	numSessions := 0
	require.NotPanics(t, func() {
		conn.handle(context.Background(), chans, func(context.Context, ssh.Channel, <-chan *ssh.Request) {
			numSessions += 1
			close(chans)
			panic("This is a panic")
		})
	})

	require.Equal(t, numSessions, 1)
}

func TestUnknownChannelType(t *testing.T) {
	rejectCh := make(chan rejectCall, 1)
	newChannel := &fakeNewChannel{channelType: "unknown session", rejectCh: rejectCh}
	conn, chans := setup(1, newChannel)

	go func() {
		conn.handle(context.Background(), chans, nil)
	}()

	rejectionData := <-rejectCh
	close(rejectCh)

	expectedRejection := rejectCall{reason: ssh.UnknownChannelType, message: "unknown channel type"}
	require.Equal(t, expectedRejection, rejectionData)
}
