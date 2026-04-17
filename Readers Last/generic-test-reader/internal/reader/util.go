package reader

import (
	"fmt"
	"time"
)

func newRequestID() string {
	return fmt.Sprintf("r-%d", time.Now().UnixNano())
}
