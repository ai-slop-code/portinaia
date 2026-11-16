package server

import (
	"fmt"
	"strconv"
	"strings"
)

func revisionETag(revision int64) string { return fmt.Sprintf(`"revision-%d"`, revision) }

func parseRevisionETag(value string) (int64, bool) {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, `"revision-`) || !strings.HasSuffix(value, `"`) {
		return 0, false
	}
	digits := strings.TrimSuffix(strings.TrimPrefix(value, `"revision-`), `"`)
	if len(digits) > 1 && digits[0] == '0' {
		return 0, false
	}
	revision, err := strconv.ParseInt(digits, 10, 64)
	return revision, err == nil && revision > 0
}
