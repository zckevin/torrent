package storage

import (
	"encoding/binary"
	"sync"
	"time"
)

type atomicKey struct {
	sync.Mutex
	key uint32
}

func (a *atomicKey) get() uint32 {
	t := uint32(time.Now().UnixNano())

	a.Lock()
	defer a.Unlock()

	if t <= a.key {
		t = a.key + 1
	}
	a.key = t

	return a.key
}

func (a *atomicKey) GetBytes() []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, a.get())
	return b
}
