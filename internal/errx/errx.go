package errx

import (
	"errors"
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

// Error represents a business error with a code and message.
type Error struct {
	Code    Code   `json:"code"`
	Message string `json:"message"`
	// Identifier optionally overrides the machine-readable `code` in the JSON
	// response. Without it every error of the same HTTP class is indistinguishable
	// to a client, so a caller that needs to branch on a specific condition has
	// nothing stable to match on. Empty means "derive it from Code".
	Identifier string `json:"-"`
	// public marks a 5xx message as written for the person who will read it.
	// See answer below: a 5xx message is kept server-side unless it is set.
	public bool
}

// Error implements error interface.
func (e *Error) Error() string {
	return fmt.Sprintf("%s (%d): %s", codeToString[e.Code], e.Code, e.Message)
}

// New creates a new business error.
func New(code Code, message string) *Error {
	return &Error{Code: code, Message: message}
}

// NewPublic creates an error whose message is shown to the caller even when
// the code is a 5xx. Use it where the fault is genuinely the reader's to act
// on: a feature this deployment has not configured, a dependency that is down
// and will come back, an operation this instance does not offer.
//
// Everything else answers a 5xx with answer5xx and keeps its own words in the
// log. See the comment there for why.
func NewPublic(code Code, message string) *Error {
	return &Error{Code: code, Message: message, public: true}
}

// NewWithIdentifier creates a business error carrying its own machine-readable
// identifier, for conditions a client is expected to detect and handle
// specifically rather than just display.
//
// An identifier is only worth minting for a condition somebody branches on,
// which is a condition somebody explains, so these are public at any status.
func NewWithIdentifier(code Code, identifier, message string) *Error {
	return &Error{Code: code, Message: message, Identifier: identifier, public: true}
}

// identifier returns the response `code`: the error's own when set, otherwise
// the generic one for its HTTP class.
func (e *Error) identifier() string {
	if e.Identifier != "" {
		return e.Identifier
	}
	if id, ok := codeToIdentifier[e.Code]; ok {
		return id
	}
	return codeToIdentifier[Internal]
}

// resolve is the HTTP status and title to answer with. A Code outside the
// table maps to 0, which gin leaves at 200, so an error would be reported as a
// success; anything unknown is an internal error instead.
func (e *Error) resolve() (int, string) {
	if status, ok := codeToHTTP[e.Code]; ok {
		return status, codeToString[e.Code]
	}
	return codeToHTTP[Internal], codeToString[Internal]
}

// ResponseCode is the machine-readable `code` this error answers with, for
// callers that embed errors in a body of their own (per-row results).
func (e *Error) ResponseCode() string { return e.identifier() }

// UserMessage is what this error says to the person who hit it: its own
// message, or the answer a server-side fault gives in place of one written for
// a stack trace.
//
// Use it wherever an error reaches a person outside the JSON envelope: a
// streamed agent event, a redirect carrying a reason, a rendered page. Those
// paths bypass JSON and Handle, so without this they are where the call site's
// own words still leak out.
func (e *Error) UserMessage() string {
	if status, _ := e.resolve(); status >= 500 && !e.public {
		return answer5xx(e.Code)
	}
	return e.Message
}

// --- Predefined errors (exported) ---
var (
	ErrUnauthorized  = New(Unauthorized, "Token not found.")
	ErrForbidden     = New(Forbidden, "You don't have access to this feature.")
	ErrNotFound      = New(NotFound, "Resource not found.")
	ErrConflict      = New(Conflict, "resource already exists")
	ErrUnprocessable = New(Unprocessable, "validation failed")
	ErrServiceDown   = New(ServiceUnavailable, "service unavailable")
)

// --- Gin handler helper ---
type response struct {
	Error     string `json:"error"`
	Message   string `json:"message"`
	Code      string `json:"code"`
	RequestID string `json:"request_id,omitempty"`
}

func InternalError() *Error {
	return New(Internal, "Something went wrong.")
}

// answer5xx is what a server-side fault says to the person who hit it.
//
// A 5xx message written at the call site is written for whoever is reading the
// stack trace: "failed to get organization count", "failed to attach roles".
// Nearly four hundred of those reach the dashboard as the explanation under a
// failed action, where they tell the reader nothing they can act on and name
// internals they should not have to know. So the message on the wire says what
// is true and what to do, and the call site's own words go to the log next to
// the request id that identifies the very same failure.
//
// The id is not repeated in the text: it is its own field, and every surface
// that renders one of these already shows it (buildError in the dashboard,
// the CLI's api.Error, the response envelope itself).
//
// A 5xx that genuinely is the reader's to act on says so with NewPublic (or by
// carrying its own identifier) and is passed through untouched: a mail
// transport nobody configured and an AI provider with no key are both faults
// only the person reading can fix.
func answer5xx(code Code) string {
	switch code {
	case ServiceUnavailable:
		return "This part of Warmbly is temporarily unavailable. Nothing was changed. Try again in a moment."
	case NotImplemented:
		return "This instance doesn't offer that."
	default:
		return "Something went wrong on our end. Try again in a moment. If it keeps happening, contact support with the request id."
	}
}

// answer is the body for one error: the status, the title, the message the
// caller is given, and the message the log keeps.
//
// detail is empty unless the two differ, so a log line only ever carries the
// call site's own words when they were not the ones sent.
func (e *Error) answer(requestID string) (int, response, string) {
	httpCode, httpError := e.resolve()
	message, detail := e.Message, ""
	if httpCode >= 500 && !e.public {
		message, detail = answer5xx(e.Code), e.Message
	}
	return httpCode, response{
		Error:     httpError,
		Message:   message,
		Code:      e.identifier(),
		RequestID: requestID,
	}, detail
}

// send writes the answer and, when the caller was given a different message
// than the call site wrote, logs the one it wrote. Nothing is lost by keeping
// a 5xx message server-side; it is only moved.
func send(c *gin.Context, e *Error) {
	requestID := c.GetString("request_id")
	httpCode, body, detail := e.answer(requestID)
	if detail != "" {
		entry := log.Error().
			Str("request_id", requestID).
			Int("status", httpCode).
			Str("code", body.Code)
		// A context built without a request is a test's or an internal
		// caller's; the detail is still worth logging without the route.
		if c.Request != nil {
			entry = entry.Str("method", c.Request.Method).Str("path", c.FullPath())
		}
		entry.Msg(detail)
	}
	c.JSON(httpCode, body)
}

func Handle(c *gin.Context, err error) {
	var bizErr *Error
	if errors.As(err, &bizErr) {
		send(c, bizErr)
		return
	}

	// Unexpected error → treat as internal, keeping what it said for the log.
	// A nil error reaching here is a caller bug rather than a request fault,
	// and answering it with a panic helps nobody.
	message := "no error given to errx.Handle"
	if err != nil {
		message = err.Error()
	}
	send(c, &Error{Code: Internal, Message: message})
}

// JSON sends a business error as JSON response
func JSON(c *gin.Context, err *Error) {
	send(c, err)
}
