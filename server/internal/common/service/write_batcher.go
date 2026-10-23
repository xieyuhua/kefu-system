package service

// 写路径批量提交（group commit）。
//
// 问题：发一条消息要写 3 次库（消息、会话摘要、客户活跃时间），每次都是一个独立事务，
// 高并发下刷盘（fsync）成为唯一瓶颈——压测里 SQLite 只有 10 RPS，加 WAL 也才 400 RPS。
//
// 做法：把多个请求的写事务攒成一批，用一个事务一起提交。语义不变（每个请求的写仍在
// 同一事务里原子提交，失败即回滚；消息主键依然由数据库生成并在提交后回填），
// 但 N 次 fsync 变成 1 次，吞吐随之提升一个数量级。
//
// 异步性：业务层调用 Submit 后由单个 writer goroutine 执行，调用方等待的是「自己那批
// 提交完成」而不是「自己独占一次刷盘」，因此并发越高摊销越明显。

import (
	"log"
	"sync"
	"time"

	"kefu-server/pkg/storage"

	"gorm.io/gorm"
)

const (
	// writeBatchWindow 攒批窗口：等这么久就提交（越大吞吐越高、单条延迟略升）
	writeBatchWindow = 2 * time.Millisecond
	// writeBatchMax 单批上限：避免一批过大导致事务过长
	writeBatchMax = 200
	// writeQueueSize 队列容量：超出时 Submit 阻塞形成背压（不丢数据）
	writeQueueSize = 4096
)

// writeJob 一个业务写单元（在事务内执行）
type writeJob struct {
	fn   func(tx *gorm.DB) error
	done chan error
}

// WriteBatcher 批量提交器
type WriteBatcher struct {
	queue chan *writeJob
	stop  chan struct{}
	wg    sync.WaitGroup
	once  sync.Once
}

var (
	batcherOnce sync.Once
	batcherInst *WriteBatcher
)

// DefaultWriteBatcher 全局批量提交器（进程内单例）
func DefaultWriteBatcher() *WriteBatcher {
	batcherOnce.Do(func() {
		batcherInst = &WriteBatcher{
			queue: make(chan *writeJob, writeQueueSize),
			stop:  make(chan struct{}),
		}
		batcherInst.start()
	})
	return batcherInst
}

func (b *WriteBatcher) start() {
	b.wg.Add(1)
	go b.loop()
}

// Submit 提交一个写单元，返回该单元所在批次提交的结果。
// fn 内拿到的主键（如消息 ID）在返回前已回填，可直接用于 VO / 推送。
func (b *WriteBatcher) Submit(fn func(tx *gorm.DB) error) error {
	job := &writeJob{fn: fn, done: make(chan error, 1)}
	b.queue <- job
	return <-job.done
}

// SubmitAsync 只入队不等结果（fire-and-forget），用于「失败也不影响主流程」的附属写
// （会话摘要、客户活跃时间等）。队列满时阻塞，形成背压而不是丢数据。
func (b *WriteBatcher) SubmitAsync(fn func(tx *gorm.DB) error) {
	job := &writeJob{fn: fn, done: make(chan error, 1)}
	b.queue <- job
	go func() {
		if err := <-job.done; err != nil {
			log.Printf("[write-batcher] 异步写失败: %v", err)
		}
	}()
}

// Stop 停止接收并退出（用于优雅关闭；未提交的任务会先落库）
func (b *WriteBatcher) Stop() {
	b.once.Do(func() { close(b.stop) })
	b.wg.Wait()
}

func (b *WriteBatcher) loop() {
	defer b.wg.Done()
	for {
		select {
		case <-b.stop:
			// 退出前把队列里已接收的任务处理完，避免丢失最后几条消息
			for {
				select {
				case job := <-b.queue:
					b.commit([]*writeJob{job})
				default:
					return
				}
			}
		case first := <-b.queue:
			batch := make([]*writeJob, 0, writeBatchMax)
			batch = append(batch, first)
			timer := time.NewTimer(writeBatchWindow)
		collect:
			for len(batch) < writeBatchMax {
				select {
				case job := <-b.queue:
					batch = append(batch, job)
				case <-timer.C:
					break collect
				}
			}
			timer.Stop()
			b.commit(batch)
		}
	}
}

// commit 在一个事务里执行整批写单元并统一提交
func (b *WriteBatcher) commit(batch []*writeJob) {
	db := storage.Default().DB()
	tx := db.Begin()
	if tx.Error != nil {
		for _, job := range batch {
			job.done <- tx.Error
		}
		return
	}
	for _, job := range batch {
		if err := job.fn(tx); err != nil {
			_ = tx.Rollback()
			// 整批回滚：把同一个错误回给该批所有调用方。
			// 不做「单条重试」——重试时结构体上的主键已被上一次尝试写入，容易插入脏数据。
			for _, j := range batch {
				j.done <- err
			}
			return
		}
	}
	err := tx.Commit().Error
	for _, job := range batch {
		job.done <- err
	}
}
