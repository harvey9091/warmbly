package repository

import (
	"strconv"

	"github.com/warmbly/warmbly/internal/utils/paging"
)

// The admin lists page by offset: their sorts are operator-chosen and often
// nullable, so no single keyset fits them.

// adminLimitOffset is the tail of a paged admin query. limitParam is the
// placeholder carrying limit+1; the offset is an int, so it is rendered inline
// and the count queries can keep dropping only the trailing LIMIT arg.
func adminLimitOffset(limitParam string, offset int) string {
	return "LIMIT " + limitParam + " OFFSET " + strconv.Itoa(offset)
}

// adminNextCursor is the cursor for the page after one that started at offset.
func adminNextCursor(offset, limit int) *string {
	return paging.EncodeOffset(offset + limit)
}
