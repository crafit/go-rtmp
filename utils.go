package rtmp

import (
	"time"
)

func GetCurrentTime() uint32 {
	t := time.Now()
	return uint32(t.Unix())
}
