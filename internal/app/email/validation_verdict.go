package email

import (
	"fmt"
	"strings"

	"github.com/warmbly/warmbly/internal/errx"
	"github.com/warmbly/warmbly/internal/models"
)

// Identifiers for a refused connect, one per kind of failure. The message
// names the leg, the host and what the server said; the code is what a client
// branches on.
const (
	ErrIDMailboxAuthRefused    = "mailbox_auth_refused"
	ErrIDMailboxUnreachable    = "mailbox_unreachable"
	ErrIDMailboxTLSFailed      = "mailbox_tls_failed"
	ErrIDMailboxServerDeclined = "mailbox_server_declined"
)

// reasonRank orders the reasons by how much they say. When the two legs fail
// differently, the code comes from the one that says the most: a refused
// password outranks a timeout on the other leg, because the timeout is not
// what the person has to fix first.
var reasonRank = map[string]int{
	models.MailProbeAuthRefused: 6,
	models.MailProbeTLS:         5,
	models.MailProbeUnreachable: 4,
	models.MailProbeCleartext:   3,
	models.MailProbeProtocol:    2,
	models.MailProbeTemporary:   1,
	models.MailProbeTimeout:     0,
}

// validationError turns a worker's verdict into the error the caller sees.
// creds carries the hosts and ports the verdict is about.
func validationError(v models.EmailValidationVerdict, creds *models.SmtpImap) *errx.Error {
	if creds == nil {
		creds = &models.SmtpImap{}
	}
	var legs []failedLeg
	if !v.SMTP.OK {
		legs = append(legs, failedLeg{"SMTP", creds.SMTP, v.SMTP})
	}
	if !v.IMAP.OK {
		legs = append(legs, failedLeg{"IMAP", creds.IMAP, v.IMAP})
	}
	if len(legs) == 0 {
		// A verdict that says "not ok" with both legs passing is malformed;
		// the legacy answer is the honest one.
		return errx.ErrEmailCredentials
	}

	lead := legs[0]
	for _, l := range legs[1:] {
		if reasonRank[l.res.Reason] > reasonRank[lead.res.Reason] {
			lead = l
		}
	}
	if lead.res.Reason == models.MailProbeTimeout {
		return timeoutError(v, creds, legs)
	}

	parts := make([]string, 0, len(legs))
	for _, l := range legs {
		parts = append(parts, legSentence(l.leg, l.svc, l.res))
	}
	msg := strings.Join(parts, " ") + " Nothing was saved."
	if hint := providerHint(lead); hint != "" {
		msg += " " + hint
	}

	id := ErrIDMailboxServerDeclined
	switch lead.res.Reason {
	case models.MailProbeAuthRefused:
		id = ErrIDMailboxAuthRefused
	case models.MailProbeUnreachable:
		id = ErrIDMailboxUnreachable
	case models.MailProbeTLS:
		id = ErrIDMailboxTLSFailed
	}
	return errx.NewWithIdentifier(errx.BadRequest, id, msg)
}

// timeoutError is the verdict where the leg that says the most stayed silent.
// It names that leg and says whether the other one got through, because a
// port that hangs while the other passes is a network in the way, not a
// password: many hosts block outbound 465 and leave 587 open.
func timeoutError(v models.EmailValidationVerdict, creds *models.SmtpImap, failed []failedLeg) *errx.Error {
	parts := make([]string, 0, 2)
	for _, l := range []failedLeg{{"SMTP", creds.SMTP, v.SMTP}, {"IMAP", creds.IMAP, v.IMAP}} {
		if l.res.OK {
			parts = append(parts, legWhere(l.leg, l.svc)+" signed in.")
			continue
		}
		parts = append(parts, legSentence(l.leg, l.svc, l.res))
	}
	msg := strings.Join(parts, " ") + " Nothing was saved. Check the host and port, then try again."
	for _, l := range failed {
		if l.leg == "SMTP" && l.res.Reason == models.MailProbeTimeout && l.svc != nil && l.svc.Port == 465 {
			msg += " Some networks block outbound port 465: if the server also offers 587, connect with 587 and STARTTLS."
			break
		}
	}
	return errx.NewWithIdentifier(errx.BadRequest, errx.ErrEmailValidation.Identifier, msg)
}

// legWhere names a leg by its server when the caller gave one.
func legWhere(leg string, svc *models.Service) string {
	if svc == nil {
		return leg + " server"
	}
	return fmt.Sprintf("%s (%s)", leg, models.MailDialAddress(models.NormalizeMailHost(svc.Host), svc.Port))
}

// legSentence is one leg's failure in the server's own words.
func legSentence(leg string, svc *models.Service, res models.EmailValidationLeg) string {
	where := legWhere(leg, svc)
	detail := ""
	if res.Detail != "" {
		detail = ": " + res.Detail
	}
	switch res.Reason {
	case models.MailProbeAuthRefused:
		return fmt.Sprintf("%s refused the sign-in%s.", where, detail)
	case models.MailProbeUnreachable:
		return fmt.Sprintf("%s could not be reached from the worker%s.", where, detail)
	case models.MailProbeTLS:
		return fmt.Sprintf("%s did not complete a secure connection%s.", where, detail)
	case models.MailProbeTemporary:
		return fmt.Sprintf("%s declined the sign-in for now and asks for a retry%s.", where, detail)
	case models.MailProbeCleartext:
		return fmt.Sprintf("%s cannot be used without encryption%s.", where, detail)
	case models.MailProbeTimeout:
		return fmt.Sprintf("%s did not answer in time.", where)
	default:
		return fmt.Sprintf("%s did not accept the sign-in%s.", where, detail)
	}
}

// failedLeg is one side of a verdict that did not pass, with the service it
// was about.
type failedLeg struct {
	leg string
	svc *models.Service
	res models.EmailValidationLeg
}

// providerHint adds the one thing worth saying about a well-known provider.
func providerHint(lead failedLeg) string {
	if lead.res.Reason != models.MailProbeAuthRefused || lead.svc == nil {
		return ""
	}
	if models.GoogleMailHost(lead.svc.Host) {
		return "Google accepts only a 16-letter app password here, created at myaccount.google.com/apppasswords on the same Google account as the address, with the username being the full address. The account password does not work, and on Google Workspace the administrator can switch IMAP or app passwords off."
	}
	return ""
}
