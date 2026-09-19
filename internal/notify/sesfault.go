package notify

import (
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/sesv2/types"
	"github.com/warmbly/warmbly/internal/observability/errs"
)

// SES refuses a send for reasons that are not interchangeable, and the raw
// operation error says which only if you already know what to look for:
//
//	operation error SESv2: SendEmail, https response error StatusCode: 400,
//	MessageRejected: Email address is not verified. The following identities
//	failed the check in region US-EAST-1: someone@example.com
//
// That is not a bad address and not an outage. It is this deployment's SES
// account still being in the sandbox, where every recipient has to be verified
// first, and it means no account on this instance can be signed up for or
// recover a password until someone requests production access. Nothing in the
// message says that, and the person who can act on it is the operator reading
// the log rather than the person who just typed their email in.
//
// So every SES refusal is reported with the one sentence that names the fix.

// sesHint is the operator-facing explanation for a refusal, empty when there
// is nothing specific to add.
func sesHint(err error) string {
	var rejected *types.MessageRejected
	if errors.As(err, &rejected) && strings.Contains(strings.ToLower(err.Error()), "not verified") {
		return "this SES account is still in the sandbox, where only verified addresses can be mailed; request production access in the SES console, or verify the recipient"
	}
	var paused *types.AccountSuspendedException
	if errors.As(err, &paused) {
		return "SES has suspended sending for this account; check the account's reputation dashboard"
	}
	var sendingPaused *types.SendingPausedException
	if errors.As(err, &sendingPaused) {
		return "SES has paused sending for this account; re-enable it in the SES console once the cause is resolved"
	}
	var throttled *types.TooManyRequestsException
	if errors.As(err, &throttled) {
		return "SES is throttling this account; the send can be retried"
	}
	var notFound *types.NotFoundException
	if errors.As(err, &notFound) {
		return "the configured sending identity does not exist in this region; check MAIL_FROM and the SES region"
	}
	return ""
}

// reportSendFailure reports one SES refusal with the sentence naming its fix.
// The caller still gets the error.
func reportSendFailure(err error, operation string) error {
	hint := sesHint(err)
	opts := []errs.Option{errs.Tag("notify.operation", operation)}
	if hint != "" {
		opts = append(opts, errs.Extra("notify.fix", hint))
		err = fmt.Errorf("%w (%s)", err, hint)
	}
	errs.CaptureException(err, opts...)
	return err
}
