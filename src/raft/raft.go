package raft

import "time"

const (
	// EnqueueLimit caps how long we will wait to enqueue
	// a new Raft command. Something is probably wrong if this
	// value is ever reached. However, it prevents us from blocking
	// the requesting goroutine forever.
	EnqueueLimit = 30 * time.Second
)
