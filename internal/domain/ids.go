package domain

import (
	"fmt"
	"time"
)

func NewID(prefix string) string { return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano()) }
