package middleware

import (
	"fmt"
	"net/http"
	"runtime/debug"

	"github.com/gin-gonic/gin"
	"github.com/warmbly/warmbly/internal/observability/errs"
)

// panicStackLimit bounds the stack attached to a report. A goroutine dump can
// run to tens of kilobytes and the answer is always in the first few frames.
const panicStackLimit = 8 << 10

// Recovery replaces gin's own so a panicking request is reported rather than
// only logged.
//
// It is what makes an error searchable back to where it happened: the report
// carries the route, the request id the client was handed, and the workspace
// and user whose request it was, so an issue in the dashboard's own error
// tracking and a support message quoting a request id are the same incident.
//
// The stack travels as an extra rather than as the event's own frames. A
// deferred recover runs after the panicking frames have unwound, so the stack
// the SDK can collect here names this middleware; debug.Stack still holds the
// real one, which is the same stack gin logs.
func Recovery() gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, recovered any) {
		stack := debug.Stack()
		if len(stack) > panicStackLimit {
			stack = stack[:panicStackLimit]
		}

		opts := []errs.Option{errs.Extra("stack", string(stack))}
		for key, value := range requestTags(c) {
			opts = append(opts, errs.Tag(key, value))
		}
		errs.Recover(recovered, opts...)

		c.AbortWithStatus(http.StatusInternalServerError)
	})
}

// requestTags is what a report should say about the request it came from.
//
// Only ids: no header, no body, no query string. A route pattern rather than
// the path, so /campaigns/:id is one issue instead of one per campaign.
func requestTags(c *gin.Context) map[string]string {
	tags := map[string]string{
		"http_method": c.Request.Method,
		"http_route":  routeOf(c),
	}
	if id := c.GetString(RequestIDContextKey); id != "" {
		tags["request_id"] = id
	}
	if v, ok := c.Get(OrganizationIDKey); ok {
		tags["organization_id"] = fmt.Sprintf("%v", v)
	}
	if id := c.GetString(UserIDKey); id != "" {
		tags["user_id"] = id
	}
	return tags
}

// routeOf is the matched route pattern, falling back to "unmatched" rather than
// to the raw path: a 404's path is attacker-supplied and would make one issue
// per probe.
func routeOf(c *gin.Context) string {
	if route := c.FullPath(); route != "" {
		return route
	}
	return "unmatched"
}
