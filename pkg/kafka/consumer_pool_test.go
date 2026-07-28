package kafka

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	segmentkafka "github.com/segmentio/kafka-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePoolWorkers(t *testing.T) {
	t.Run("未配置默认3", func(t *testing.T) {
		n, err := ParsePoolWorkers("")
		require.NoError(t, err)
		assert.Equal(t, DefaultPoolWorkers, n)
	})
	t.Run("空白等同未配置", func(t *testing.T) {
		n, err := ParsePoolWorkers("  \t  ")
		require.NoError(t, err)
		assert.Equal(t, DefaultPoolWorkers, n)
	})
	t.Run("显式合法值", func(t *testing.T) {
		for _, tc := range []struct {
			raw string
			n   int
		}{
			{"1", 1},
			{"3", 3},
			{"7", 7},
			{"64", 64},
		} {
			n, err := ParsePoolWorkers(tc.raw)
			require.NoError(t, err, tc.raw)
			assert.Equal(t, tc.n, n, tc.raw)
		}
	})
	t.Run("非法值直接报错", func(t *testing.T) {
		for _, raw := range []string{"0", "-1", "-3", "65", "100", "abc", "3.5", "1e2", "not-a-number"} {
			_, err := ParsePoolWorkers(raw)
			require.Error(t, err, raw)
		}
	})
}

func TestNewManualConsumerPoolCreatesNDistinctConsumers(t *testing.T) {
	pool, err := NewManualConsumerPool(ManualConsumerPoolConfig{
		Name:    "test-projector",
		Brokers: []string{"127.0.0.1:9092"},
		Topic:   "group.cache",
		GroupID: "group-cache-projector-group",
		Workers: 3,
		Config: ManualConsumerConfig{
			MinBytes:     1,
			MaxBytes:     1024,
			MaxWait:      10 * time.Millisecond,
			ErrorBackoff: time.Millisecond,
		},
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = pool.Close() })

	assert.Equal(t, 3, pool.WorkerCount())
	workers := pool.Workers()
	require.Len(t, workers, 3)
	// 必须是三个不同 Consumer 实例（各自持有 Reader），不能共享。
	assert.True(t, workers[0] != workers[1] && workers[1] != workers[2] && workers[0] != workers[2])
	assert.True(t, workers[0].reader != workers[1].reader)
}

func TestNewManualConsumerPoolRejectsInvalidWorkers(t *testing.T) {
	_, err := NewManualConsumerPool(ManualConsumerPoolConfig{
		Name:    "x",
		Brokers: []string{"127.0.0.1:9092"},
		Topic:   "t",
		GroupID: "g",
		Workers: 0,
	})
	require.Error(t, err)
}

// blockingReader 可控的假 Reader：按脚本返回消息，并记录 commit/close。
type blockingReader struct {
	mu          sync.Mutex
	fetchCalls  int
	fetchScript []func(ctx context.Context, call int) (segmentkafka.Message, error)
	committed   []segmentkafka.Message
	closeCalls  atomic.Int32
	closeErr    error
	// afterCommit 在每次成功 commit 后调用，用于串行断言。
	afterCommit func(msg segmentkafka.Message)
}

func (r *blockingReader) FetchMessage(ctx context.Context) (segmentkafka.Message, error) {
	r.mu.Lock()
	r.fetchCalls++
	call := r.fetchCalls
	var fn func(context.Context, int) (segmentkafka.Message, error)
	if call-1 < len(r.fetchScript) {
		fn = r.fetchScript[call-1]
	}
	r.mu.Unlock()
	if fn != nil {
		return fn(ctx, call)
	}
	// 脚本耗尽后阻塞到 ctx 取消。
	<-ctx.Done()
	return segmentkafka.Message{}, ctx.Err()
}

func (r *blockingReader) CommitMessages(_ context.Context, msgs ...segmentkafka.Message) error {
	r.mu.Lock()
	r.committed = append(r.committed, msgs...)
	cb := r.afterCommit
	r.mu.Unlock()
	if cb != nil {
		for _, m := range msgs {
			cb(m)
		}
	}
	return nil
}

func (r *blockingReader) Close() error {
	r.closeCalls.Add(1)
	return r.closeErr
}

func TestManualConsumerPool_TwoWorkersEnterHandlerConcurrently(t *testing.T) {
	var entered atomic.Int32
	bothIn := make(chan struct{})
	var once sync.Once

	handlerBlock := make(chan struct{})
	handler := func(ctx context.Context, _ []byte) error {
		n := entered.Add(1)
		if n == 2 {
			once.Do(func() { close(bothIn) })
		}
		select {
		case <-handlerBlock:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	mkReader := func(payload string) *blockingReader {
		return &blockingReader{
			fetchScript: []func(context.Context, int) (segmentkafka.Message, error){
				func(ctx context.Context, _ int) (segmentkafka.Message, error) {
					return segmentkafka.Message{Topic: "group.cache", Value: []byte(payload)}, nil
				},
			},
		}
	}
	r0 := mkReader("a")
	r1 := mkReader("b")
	c0 := &Consumer{reader: r0, commitMode: CommitOnSuccess, handleTimeout: -1}
	c1 := &Consumer{reader: r1, commitMode: CommitOnSuccess, handleTimeout: -1}

	pool, err := newManualConsumerPoolForTest("test-projector", "group.cache", "g1", []*Consumer{c0, c1})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- pool.Start(ctx, handler) }()

	select {
	case <-bothIn:
		// 两个 worker 同时进入 handler，证明不同 Reader 并行。
	case <-time.After(2 * time.Second):
		t.Fatal("两个 worker 未在超时内同时进入 handler")
	}
	assert.Equal(t, int32(2), entered.Load())

	close(handlerBlock)
	cancel()
	select {
	case err := <-errCh:
		require.Error(t, err)
		assert.True(t, errors.Is(err, context.Canceled))
	case <-time.After(2 * time.Second):
		t.Fatal("pool.Start 未在取消后返回")
	}
}

func TestManualConsumerPool_SameWorkerSerialUntilCommit(t *testing.T) {
	// 同一 worker：第二条消息只有在第一条 Commit 之后才会被 Fetch。
	var phase atomic.Int32 // 0=等待 msg1 handle, 1=msg1 committed, 2=msg2 fetched
	msg1Done := make(chan struct{})
	msg2Fetched := make(chan struct{})

	reader := &blockingReader{
		fetchScript: []func(context.Context, int) (segmentkafka.Message, error){
			func(context.Context, int) (segmentkafka.Message, error) {
				return segmentkafka.Message{Topic: "t", Partition: 0, Offset: 1, Value: []byte("m1")}, nil
			},
			func(ctx context.Context, _ int) (segmentkafka.Message, error) {
				// 必须在 msg1 提交后才允许第二次 fetch。
				if phase.Load() < 1 {
					t.Error("同一 worker 在前一条 commit 前就 Fetch 了下一条")
				}
				close(msg2Fetched)
				phase.Store(2)
				<-ctx.Done()
				return segmentkafka.Message{}, ctx.Err()
			},
		},
		afterCommit: func(msg segmentkafka.Message) {
			if msg.Offset == 1 {
				phase.Store(1)
				close(msg1Done)
			}
		},
	}

	var handleOrder []string
	var handleMu sync.Mutex
	handler := func(_ context.Context, b []byte) error {
		handleMu.Lock()
		handleOrder = append(handleOrder, string(b))
		handleMu.Unlock()
		return nil
	}

	c := &Consumer{reader: reader, commitMode: CommitOnSuccess, handleTimeout: -1, errorBackoff: time.Millisecond}
	pool, err := newManualConsumerPoolForTest("serial", "t", "g", []*Consumer{c})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- pool.Start(ctx, handler) }()

	select {
	case <-msg1Done:
	case <-time.After(2 * time.Second):
		t.Fatal("msg1 未提交")
	}
	select {
	case <-msg2Fetched:
	case <-time.After(2 * time.Second):
		t.Fatal("msg1 提交后未 Fetch msg2")
	}

	cancel()
	<-errCh
	handleMu.Lock()
	assert.Equal(t, []string{"m1"}, handleOrder)
	handleMu.Unlock()
	require.Len(t, reader.committed, 1)
	assert.Equal(t, int64(1), reader.committed[0].Offset)
}

func TestManualConsumerPool_WorkerFatalCancelsOthers(t *testing.T) {
	// worker0 对无死信的 Permanent 错误会让 consumeOnSuccess 返回致命错误；
	// worker1 应被 cancel，不得继续空转。
	fatalSeen := make(chan struct{})
	var once sync.Once

	r0 := &blockingReader{
		fetchScript: []func(context.Context, int) (segmentkafka.Message, error){
			func(context.Context, int) (segmentkafka.Message, error) {
				return segmentkafka.Message{Topic: "t", Value: []byte("boom")}, nil
			},
		},
	}
	r1 := &blockingReader{}
	var worker1Canceled atomic.Bool
	r1.fetchScript = []func(context.Context, int) (segmentkafka.Message, error){
		func(ctx context.Context, _ int) (segmentkafka.Message, error) {
			<-ctx.Done()
			worker1Canceled.Store(true)
			return segmentkafka.Message{}, ctx.Err()
		},
	}

	c0 := &Consumer{reader: r0, commitMode: CommitOnSuccess, handleTimeout: -1, errorBackoff: time.Millisecond}
	c1 := &Consumer{reader: r1, commitMode: CommitOnSuccess, handleTimeout: -1, errorBackoff: time.Millisecond}
	pool, err := newManualConsumerPoolForTest("fatal-test", "t", "g", []*Consumer{c0, c1})
	require.NoError(t, err)

	handler := func(context.Context, []byte) error {
		once.Do(func() { close(fatalSeen) })
		return Permanent(errors.New("bad payload"))
	}

	errCh := make(chan error, 1)
	go func() { errCh <- pool.Start(context.Background(), handler) }()

	select {
	case <-fatalSeen:
	case <-time.After(2 * time.Second):
		t.Fatal("worker0 未进入 handler")
	}

	select {
	case err := <-errCh:
		require.Error(t, err)
		assert.Contains(t, err.Error(), "致命退出")
		assert.True(t, worker1Canceled.Load(), "worker0 致命后应取消 worker1")
	case <-time.After(2 * time.Second):
		t.Fatal("pool 未在 worker 致命后收敛")
	}
}

func TestManualConsumerPool_CloseClosesAllReadersAndAggregatesErrors(t *testing.T) {
	r0 := &blockingReader{closeErr: errors.New("close-0")}
	r1 := &blockingReader{}
	r2 := &blockingReader{closeErr: errors.New("close-2")}
	pool, err := newManualConsumerPoolForTest("close-test", "t", "g", []*Consumer{
		{reader: r0},
		{reader: r1},
		{reader: r2},
	})
	require.NoError(t, err)

	err = pool.Close()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "close-0")
	assert.Contains(t, err.Error(), "close-2")
	assert.Equal(t, int32(1), r0.closeCalls.Load())
	assert.Equal(t, int32(1), r1.closeCalls.Load())
	assert.Equal(t, int32(1), r2.closeCalls.Load())

	// 二次 Close 不重复关闭。
	require.Error(t, pool.Close())
	assert.Equal(t, int32(1), r0.closeCalls.Load())
}

func TestManualConsumerPool_ContextCancelDoesNotCommitFailedMessage(t *testing.T) {
	// handler 返回可重试错误；在重试退避期间取消 ctx，不应提交。
	var handleCalls atomic.Int32
	reader := &blockingReader{
		fetchScript: []func(context.Context, int) (segmentkafka.Message, error){
			func(context.Context, int) (segmentkafka.Message, error) {
				return segmentkafka.Message{Topic: "t", Offset: 9, Value: []byte("x")}, nil
			},
		},
	}
	c := &Consumer{
		reader:         reader,
		commitMode:     CommitOnSuccess,
		handleTimeout:  -1,
		errorBackoff:   50 * time.Millisecond,
		retryBudget:    time.Hour,
		deadLetterSink: nil, // 无死信：可重试错误原地重试
	}
	pool, err := newManualConsumerPoolForTest("cancel-test", "t", "g", []*Consumer{c})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	handler := func(context.Context, []byte) error {
		n := handleCalls.Add(1)
		if n == 1 {
			go func() {
				time.Sleep(10 * time.Millisecond)
				cancel()
			}()
		}
		return errors.New("temporary")
	}

	err = pool.Start(ctx, handler)
	require.Error(t, err)
	assert.True(t, errors.Is(err, context.Canceled))
	assert.Empty(t, reader.committed, "context 取消不得提交当前失败消息的 offset")
	assert.GreaterOrEqual(t, handleCalls.Load(), int32(1))
}

func TestManualConsumerPool_NilSafeClose(t *testing.T) {
	var p *ManualConsumerPool
	require.NoError(t, p.Close())
	p = &ManualConsumerPool{name: "partial"}
	require.NoError(t, p.Close())
}
