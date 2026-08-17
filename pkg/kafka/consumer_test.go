package kafka

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	segmentkafka "github.com/segmentio/kafka-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type consumerTestReader struct {
	commitErrors []error
	commitCalls  int
	committed    []segmentkafka.Message
}

func (r *consumerTestReader) FetchMessage(context.Context) (segmentkafka.Message, error) {
	return segmentkafka.Message{}, errors.New("unexpected FetchMessage call")
}

func (r *consumerTestReader) CommitMessages(_ context.Context, messages ...segmentkafka.Message) error {
	r.commitCalls++
	if len(r.commitErrors) >= r.commitCalls && r.commitErrors[r.commitCalls-1] != nil {
		return r.commitErrors[r.commitCalls-1]
	}
	r.committed = append(r.committed, messages...)
	return nil
}

func (r *consumerTestReader) Close() error { return nil }

type consumerTestDeadLetterSink struct {
	records []DeadLetterRecord
	err     error
}

func (s *consumerTestDeadLetterSink) Park(_ context.Context, record DeadLetterRecord) error {
	s.records = append(s.records, record)
	return s.err
}

func TestConsumeOnSuccessParksPermanentErrorOnceBeforeRetryingCommit(t *testing.T) {
	reader := &consumerTestReader{commitErrors: []error{errors.New("temporary commit failure"), nil}}
	sink := &consumerTestDeadLetterSink{}
	consumer := &Consumer{
		reader:         reader,
		errorBackoff:   0,
		deadLetterSink: sink,
	}
	message := segmentkafka.Message{
		Topic:     "group.cache",
		Partition: 2,
		Offset:    19,
		Key:       []byte("group-1"),
		Value:     []byte(`{"broken":true}`),
	}
	handlerCalls := 0

	err := consumer.consumeOnSuccess(context.Background(), func(context.Context, []byte) error {
		handlerCalls++
		return Permanent(errors.New("schema mismatch"))
	}, message)
	require.NoError(t, err)

	assert.Equal(t, 1, handlerCalls, "commit 重试不得再次执行 handler")
	require.Len(t, sink.records, 1, "commit 重试不得重复插入死信")
	assert.Equal(t, message.Topic, sink.records[0].Topic)
	assert.Equal(t, message.Partition, sink.records[0].Partition)
	assert.Equal(t, message.Offset, sink.records[0].Offset)
	assert.Equal(t, 1, sink.records[0].Attempts)
	assert.Equal(t, 2, reader.commitCalls)
	require.Len(t, reader.committed, 1)
	assert.Equal(t, message.Offset, reader.committed[0].Offset)
}

func TestConsumeOnSuccessRejectsPermanentErrorWithoutDeadLetterSink(t *testing.T) {
	reader := &consumerTestReader{}
	consumer := &Consumer{
		reader:       reader,
		errorBackoff: time.Millisecond,
	}

	err := consumer.consumeOnSuccess(
		context.Background(),
		func(context.Context, []byte) error {
			return Permanent(errors.New("invalid payload"))
		},
		segmentkafka.Message{Topic: "group.cache", Offset: 7},
	)
	require.Error(t, err)
	assert.True(t, IsPermanent(err))
	assert.Zero(t, reader.commitCalls)
}

func TestCommitMessageTreatsClosedReaderAsFatal(t *testing.T) {
	reader := &consumerTestReader{commitErrors: []error{io.ErrClosedPipe}}
	observedErrors := 0
	consumer := &Consumer{
		reader:       reader,
		errorBackoff: time.Hour,
		observeCommitError: func() {
			observedErrors++
		},
	}

	err := consumer.commitMessage(context.Background(), segmentkafka.Message{Topic: "t", Offset: 1})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reader 已关闭")
	assert.Equal(t, 1, reader.commitCalls)
	assert.Equal(t, 1, observedErrors)
}
