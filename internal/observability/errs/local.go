package errs

import (
	"context"
	"log"
	"time"
)

// localSink is what a deployment with no reporting backend gets: the event is
// written to the process log and nothing leaves the machine. It is the
// self-host default, so this path has to stay useful on its own.
type localSink struct {
	service string
}

func (l localSink) capture(ev event) {
	if ev.err != nil {
		log.Printf("[issue-local][%s][error] %v", l.service, ev.err)
		return
	}
	log.Printf("[issue-local][%s][info] %s", l.service, ev.message)
}

func (l localSink) recoverPanic(_ context.Context, r any, _ scope) {
	log.Printf("[issue-local][%s][panic] %v", l.service, r)
}

func (localSink) flush(time.Duration) bool { return true }
