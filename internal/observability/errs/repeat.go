package errs

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"
)

// One fault that recurs is one thing to fix, and reporting every occurrence of
// it buries everything else. A schema the registry refused filed 19,190 events
// in three hours; a Redis provider over its request quota filed 2,815 in five;
// both times every other issue in the project was pushed off the first page
// while nothing was learned after the first copy.
//
// So a recurring fault reports once, then again no more often than
// repeatWindow, and the report that breaks the silence says how many it stands
// for. Nothing is thrown away silently: the count is on the event.
//
// This is deliberately in errs rather than at the call sites that flooded.
// Every one of those had a reason to report, and the next flood will come from
// a call site nobody has throttled yet.
const repeatWindow = 5 * time.Minute

// repeatKeyLimit bounds the table. A process that produces more distinct faults
// than this has a bigger problem than its reporting, and an unbounded map on
// the error path is a way to turn a bad hour into an OOM. Reaching the limit
// drops the table rather than evicting one entry, which costs at most one
// duplicate report per fault and cannot leave a stale half.
const repeatKeyLimit = 4096

type repeatState struct {
	reportedAt time.Time
	suppressed int
}

var (
	repeatMu sync.Mutex
	repeats  = map[string]*repeatState{}
)

// admit decides whether this event is sent, and annotates it with the number
// its silence covered. Called from the public entry points before report, so
// the number of frames between a caller and a sink is unchanged (see
// stackSkip).
func admit(sc *scope, key string) bool {
	if sc.always || key == "" {
		return true
	}
	now := time.Now()

	repeatMu.Lock()
	if len(repeats) >= repeatKeyLimit {
		repeats = map[string]*repeatState{}
	}
	state, seen := repeats[key]
	if !seen {
		repeats[key] = &repeatState{reportedAt: now}
		repeatMu.Unlock()
		return true
	}
	if now.Sub(state.reportedAt) < repeatWindow {
		state.suppressed++
		repeatMu.Unlock()
		return false
	}
	suppressed := state.suppressed
	state.reportedAt = now
	state.suppressed = 0
	repeatMu.Unlock()

	if suppressed > 0 {
		Extra("repeat.suppressed", suppressed)(sc)
		Extra("repeat.window", repeatWindow.String())(sc)
	}
	return true
}

// variable matches the parts of a message that differ between occurrences of
// the same fault: the ids, addresses and counters. Collapsing them is what lets
// "mailbox <uuid> could not be reached" be recognised as one recurring fault
// rather than one per mailbox.
//
// Deliberately blunt. A key that merges two faults costs one delayed report; a
// key that separates every occurrence of one fault is the flood this exists to
// stop.
var variable = regexp.MustCompile(
	`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}` + // uuid
		`|0x[0-9a-fA-F]+` + // hex literal
		`|\b\d[\d.:]*\b`, // numbers, addresses, ports, durations
)

// fingerprintLimit keeps one pathological message (a query, a stack, a body)
// from being the key it is hashed into.
const fingerprintLimit = 256

// fingerprintError keys an error by its Go type and the shape of its message.
// The type alone is far too coarse: every wrapped error in the codebase is
// *fmt.wrapError.
func fingerprintError(err error) string {
	return fmt.Sprintf("%T", err) + "\x00" + shapeOf(err.Error())
}

func shapeOf(text string) string {
	if len(text) > fingerprintLimit {
		text = text[:fingerprintLimit]
	}
	return variable.ReplaceAllString(strings.TrimSpace(text), "#")
}
