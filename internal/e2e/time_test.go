package e2e

import (
	"time"

	"github.com/google/uuid"
)

var timeNow = time.Now

func mustUUID(s string) uuid.UUID { return uuid.MustParse(s) }
