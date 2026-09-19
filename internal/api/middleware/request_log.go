package middleware

import (
	"fmt"
	"io"

	"github.com/gin-gonic/gin"
)

// RequestLogger is gin's standard access log with the query string removed.
//
// gin.Logger() prints the full request URI, and several routes carry a
// credential there because the provider or the mail client puts it there:
// OAuth `code` and `state` on the callback bouncers, the team invitation token
// on the preview lookup, the socket ticket on /v1/getaway's reply, the form
// prefill ticket. Those end up in stdout, in the container log, and in whatever
// aggregates it, which is a place none of them should reach.
//
// The path is kept because that is what the log is for. Nothing downstream
// needs the query: a request that has to be traced has X-Request-Id.
func RequestLogger() gin.HandlerFunc {
	return gin.LoggerWithConfig(gin.LoggerConfig{
		Formatter: func(p gin.LogFormatterParams) string {
			return fmt.Sprintf("[GIN] %v | %3d | %13v | %15s | %-7s %#v\n",
				p.TimeStamp.Format("2006/01/02 - 15:04:05"),
				p.StatusCode,
				p.Latency,
				p.ClientIP,
				p.Method,
				p.Path,
			)
		},
		Output: io.Writer(gin.DefaultWriter),
	})
}
