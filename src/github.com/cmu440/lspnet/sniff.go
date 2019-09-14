// DO NOT MODIFY THIS FILE!
// STUDENTS MUST NOT CALL ANY METHODS IN THIS FILE!

package lspnet

import "sync/atomic"
import "sync"

type SniffResult struct {
	NumSentACKs    int
	NumDroppedACKS int
	NumSentData    int
	NumDroppedData int
}

var isSniffing uint32 = 0
var sniffRes SniffResult
var sniffResLock sync.Mutex

func isSniff() bool {
	if atomic.LoadUint32(&isSniffing) == 0 {
		return false
	}
	return true
}

func record(msgType int, isSent bool) {
	sniffResLock.Lock()
	defer sniffResLock.Unlock()
	if msgType == TypeMsgData {
		if isSent {
			sniffRes.NumSentData++
		} else {
			sniffRes.NumDroppedData++
		}
	} else if msgType == TypeMsgAck {
		if isSent {
			sniffRes.NumSentACKs++
		} else {
			sniffRes.NumDroppedACKS++
		}
	}
}

func StartSniff() {
	sniffResLock.Lock()
	sniffRes.NumSentACKs = 0
	sniffRes.NumDroppedACKS = 0
	sniffRes.NumSentData = 0
	sniffRes.NumDroppedData = 0
	sniffResLock.Unlock()
	atomic.StoreUint32(&isSniffing, 1)
}

func StopSniff() SniffResult {
	atomic.StoreUint32(&isSniffing, 0)
	sniffResLock.Lock()
	defer sniffResLock.Unlock()
	return sniffRes
}
