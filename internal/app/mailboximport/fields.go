// Package mailboximport connects many mailboxes from one file: it reads any
// reasonable spreadsheet or pasted list, works out what each column means and
// where each domain's mail is hosted, and then connects the rows in the
// background, grouping whatever fails by cause.
package mailboximport

import (
	"strings"
	"unicode"

	"github.com/warmbly/warmbly/internal/models"
)

// normalizeHeader folds a header to letters and digits, so "SMTP Host",
// "smtp_host" and "smtpHost" meet.
func normalizeHeader(h string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(h)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// aliases maps a normalized header to a field. It covers our own template and
// the exports of the inbox vendors and sequencers people move mailboxes between.
var aliases = map[string]models.MailboxImportField{
	"email": models.ImportFieldEmail, "emailaddress": models.ImportFieldEmail, "address": models.ImportFieldEmail,
	"mailbox": models.ImportFieldEmail, "fromemail": models.ImportFieldEmail, "emailaccount": models.ImportFieldEmail,
	"senderemail": models.ImportFieldEmail, "inbox": models.ImportFieldEmail, "account": models.ImportFieldEmail,
	"mailboxemail": models.ImportFieldEmail, "emailid": models.ImportFieldEmail,

	"name": models.ImportFieldName, "displayname": models.ImportFieldName, "fromname": models.ImportFieldName,
	"sendername": models.ImportFieldName, "sendersname": models.ImportFieldName, "fullname": models.ImportFieldName,
	"smtpfromname": models.ImportFieldName,
	"firstname":    models.ImportFieldFirstName, "first": models.ImportFieldFirstName, "givenname": models.ImportFieldFirstName,
	"lastname": models.ImportFieldLastName, "last": models.ImportFieldLastName, "surname": models.ImportFieldLastName,
	"familyname": models.ImportFieldLastName,

	"password": models.ImportFieldPassword, "pass": models.ImportFieldPassword, "pwd": models.ImportFieldPassword,
	"accountpassword": models.ImportFieldPassword, "mailboxpassword": models.ImportFieldPassword,
	"emailpassword": models.ImportFieldPassword,
	"apppassword":   models.ImportFieldAppPassword, "applicationpassword": models.ImportFieldAppPassword,
	"appspecificpassword": models.ImportFieldAppPassword, "googleapppassword": models.ImportFieldAppPassword,
	"smtppassword": models.ImportFieldSMTPPassword, "smtppass": models.ImportFieldSMTPPassword,
	"imappassword": models.ImportFieldIMAPPassword, "imappass": models.ImportFieldIMAPPassword,

	"username": models.ImportFieldUsername, "user": models.ImportFieldUsername, "login": models.ImportFieldUsername,
	"loginemail":   models.ImportFieldUsername,
	"smtpusername": models.ImportFieldSMTPUsername, "smtpuser": models.ImportFieldSMTPUsername,
	"smtplogin": models.ImportFieldSMTPUsername, "smtpemail": models.ImportFieldSMTPUsername,
	"imapusername": models.ImportFieldIMAPUsername, "imapuser": models.ImportFieldIMAPUsername,
	"imaplogin": models.ImportFieldIMAPUsername, "imapemail": models.ImportFieldIMAPUsername,

	"smtphost": models.ImportFieldSMTPHost, "smtpserver": models.ImportFieldSMTPHost, "outgoingserver": models.ImportFieldSMTPHost,
	"smtpport": models.ImportFieldSMTPPort, "outgoingport": models.ImportFieldSMTPPort,
	"smtpsecurity": models.ImportFieldSMTPSecurity, "smtpencryption": models.ImportFieldSMTPSecurity,
	"smtpssl": models.ImportFieldSMTPSecurity, "smtpporttype": models.ImportFieldSMTPSecurity, "smtptls": models.ImportFieldSMTPSecurity,
	"imaphost": models.ImportFieldIMAPHost, "imapserver": models.ImportFieldIMAPHost, "incomingserver": models.ImportFieldIMAPHost,
	"imapport": models.ImportFieldIMAPPort, "incomingport": models.ImportFieldIMAPPort,
	"imapsecurity": models.ImportFieldIMAPSecurity, "imapencryption": models.ImportFieldIMAPSecurity,
	"imapssl": models.ImportFieldIMAPSecurity, "imapporttype": models.ImportFieldIMAPSecurity, "imaptls": models.ImportFieldIMAPSecurity,

	"dailylimit": models.ImportFieldDailyLimit, "maxemailperday": models.ImportFieldDailyLimit,
	"sendinglimit": models.ImportFieldDailyLimit, "dailysendinglimit": models.ImportFieldDailyLimit,
	"dailyquota": models.ImportFieldDailyLimit, "campaignlimit": models.ImportFieldDailyLimit, "limit": models.ImportFieldDailyLimit,
	"minwait": models.ImportFieldMinWait, "mingap": models.ImportFieldMinWait, "delay": models.ImportFieldMinWait,
	"minwaittime": models.ImportFieldMinWait, "timegap": models.ImportFieldMinWait,

	"warmup": models.ImportFieldWarmup, "warmupenabled": models.ImportFieldWarmup, "enablewarmup": models.ImportFieldWarmup,
	"warmupstart": models.ImportFieldWarmupStart, "warmupbase": models.ImportFieldWarmupStart,
	"warmupmax": models.ImportFieldWarmupMax, "warmuplimit": models.ImportFieldWarmupMax,
	"totalwarmupperday": models.ImportFieldWarmupMax, "warmupdailygoal": models.ImportFieldWarmupMax,
	"warmupincrease": models.ImportFieldWarmupIncrease, "warmupincrement": models.ImportFieldWarmupIncrease,
	"dailyrampup": models.ImportFieldWarmupIncrease, "warmupdailyincrement": models.ImportFieldWarmupIncrease,
	"rampup":          models.ImportFieldWarmupIncrease,
	"warmupreplyrate": models.ImportFieldWarmupReplyRate, "replyrate": models.ImportFieldWarmupReplyRate,
	"replyratepercentage": models.ImportFieldWarmupReplyRate,

	"replyto": models.ImportFieldReplyTo, "differentreplytoaddress": models.ImportFieldReplyTo,
	"replytoaddress": models.ImportFieldReplyTo, "replytoemail": models.ImportFieldReplyTo,
	"signature": models.ImportFieldSignature, "signaturehtml": models.ImportFieldSignature, "emailsignature": models.ImportFieldSignature,
	"footer": models.ImportFieldSignature,
	"tags":   models.ImportFieldTags, "tag": models.ImportFieldTags, "labels": models.ImportFieldTags, "label": models.ImportFieldTags,
	"timezone": models.ImportFieldTimezone, "tz": models.ImportFieldTimezone,
}

// secretFields never leave the server in a preview and never reach a download.
var secretFields = map[models.MailboxImportField]bool{
	models.ImportFieldPassword:     true,
	models.ImportFieldAppPassword:  true,
	models.ImportFieldSMTPPassword: true,
	models.ImportFieldIMAPPassword: true,
}

// allFields is every field a column can be mapped to, in the order the UI groups them.
var allFields = []models.MailboxImportField{
	models.ImportFieldEmail, models.ImportFieldName, models.ImportFieldFirstName, models.ImportFieldLastName,
	models.ImportFieldPassword, models.ImportFieldAppPassword, models.ImportFieldSMTPPassword, models.ImportFieldIMAPPassword,
	models.ImportFieldUsername, models.ImportFieldSMTPUsername, models.ImportFieldIMAPUsername,
	models.ImportFieldSMTPHost, models.ImportFieldSMTPPort, models.ImportFieldSMTPSecurity,
	models.ImportFieldIMAPHost, models.ImportFieldIMAPPort, models.ImportFieldIMAPSecurity,
	models.ImportFieldDailyLimit, models.ImportFieldMinWait,
	models.ImportFieldWarmup, models.ImportFieldWarmupStart, models.ImportFieldWarmupMax,
	models.ImportFieldWarmupIncrease, models.ImportFieldWarmupReplyRate,
	models.ImportFieldReplyTo, models.ImportFieldSignature, models.ImportFieldTags, models.ImportFieldTimezone,
}

// fieldDescriptions are the choices offered to Jev for a column no rule placed.
var fieldDescriptions = map[models.MailboxImportField]string{
	models.ImportFieldEmail:           "the mailbox's email address",
	models.ImportFieldName:            "the sender's full display name",
	models.ImportFieldFirstName:       "the sender's first name",
	models.ImportFieldLastName:        "the sender's last name",
	models.ImportFieldPassword:        "the mailbox account's password",
	models.ImportFieldAppPassword:     "an app-specific password generated for mail clients",
	models.ImportFieldSMTPPassword:    "the password used for the outgoing (SMTP) server only",
	models.ImportFieldIMAPPassword:    "the password used for the incoming (IMAP) server only",
	models.ImportFieldUsername:        "the login name used to sign in to the mail servers",
	models.ImportFieldSMTPUsername:    "the login name for the outgoing (SMTP) server only",
	models.ImportFieldIMAPUsername:    "the login name for the incoming (IMAP) server only",
	models.ImportFieldSMTPHost:        "the outgoing (SMTP) server hostname",
	models.ImportFieldSMTPPort:        "the outgoing (SMTP) server port number",
	models.ImportFieldSMTPSecurity:    "the outgoing server's encryption (SSL, TLS or STARTTLS)",
	models.ImportFieldIMAPHost:        "the incoming (IMAP) server hostname",
	models.ImportFieldIMAPPort:        "the incoming (IMAP) server port number",
	models.ImportFieldIMAPSecurity:    "the incoming server's encryption (SSL, TLS or STARTTLS)",
	models.ImportFieldDailyLimit:      "how many campaign emails the mailbox may send per day",
	models.ImportFieldMinWait:         "the minimum wait between two sends from the mailbox",
	models.ImportFieldWarmup:          "whether warmup is switched on (yes/no)",
	models.ImportFieldWarmupStart:     "how many warmup emails to send on the first day",
	models.ImportFieldWarmupMax:       "the most warmup emails to send per day",
	models.ImportFieldWarmupIncrease:  "how many warmup emails to add each day",
	models.ImportFieldWarmupReplyRate: "the percentage of warmup emails that get a reply",
	models.ImportFieldReplyTo:         "a reply-to email address different from the mailbox",
	models.ImportFieldSignature:       "the email signature",
	models.ImportFieldTags:            "tags or labels to put on the mailbox",
	models.ImportFieldTimezone:        "the mailbox's time zone",
	models.ImportFieldIgnore:          "something else that is not needed to connect or configure a mailbox",
}

func validField(f models.MailboxImportField) bool {
	if f == models.ImportFieldIgnore {
		return true
	}
	_, ok := fieldDescriptions[f]
	return ok
}

// vendor recognizes a file by headers only its exporter writes.
type vendor struct {
	id, label string
	// all of these normalized headers must be present
	needs []string
}

var vendors = []vendor{
	{"zapmail", "Zapmail", []string{"apppassword", "recoveryemail"}},
	{"inboxkit", "InboxKit", []string{"apppassword", "domainname"}},
	{"smartlead", "Smartlead", []string{"fromname", "fromemail", "username", "maxemailperday"}},
	{"instantly", "Instantly", []string{"imapusername", "smtpusername", "warmupenabled", "dailylimit"}},
	{"saleshandy", "Saleshandy", []string{"emailaccount", "emailserviceprovider", "sendersname"}},
}

func detectVendor(normalized map[string]bool) *models.MailboxImportVendor {
	for _, v := range vendors {
		hit := true
		for _, n := range v.needs {
			if !normalized[n] {
				hit = false
				break
			}
		}
		if hit {
			return &models.MailboxImportVendor{ID: v.id, Label: v.label}
		}
	}
	return nil
}
