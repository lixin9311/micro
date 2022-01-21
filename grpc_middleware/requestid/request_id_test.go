package request_id

import (
	"testing"

	"github.com/segmentio/ksuid"
)

func TestRequestID(t *testing.T) {
	id := ksuid.New().String()

	t.Logf("generated id: %s", id)
}
