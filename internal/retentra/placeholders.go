package retentra

import (
	"strings"
	"time"
)

type placeholders struct {
	tmpdir string
	now    time.Time
}

func (p placeholders) expand(value string) string {
	value = strings.ReplaceAll(value, "{tmpdir}", p.tmpdir)
	value = strings.ReplaceAll(value, "{date}", p.now.Format("2006-01-02"))
	return value
}
