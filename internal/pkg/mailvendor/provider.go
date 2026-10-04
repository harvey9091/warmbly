package mailvendor

import "strings"

// normalizeProvider maps a vendor's platform label to a Mailbox.Provider value.
func normalizeProvider(s string) string {
	v := strings.ToLower(strings.TrimSpace(s))
	switch {
	case v == "":
		return ""
	case strings.Contains(v, "google"), strings.Contains(v, "gsuite"), strings.Contains(v, "g suite"),
		strings.Contains(v, "workspace"), strings.Contains(v, "gmail"):
		return ProviderGoogle
	case strings.Contains(v, "outlook"), strings.Contains(v, "microsoft"), strings.Contains(v, "office365"),
		strings.Contains(v, "office 365"), strings.Contains(v, "m365"), strings.Contains(v, "o365"),
		strings.Contains(v, "azure"), strings.Contains(v, "exchange"):
		return ProviderMicrosoft
	case v == "smtp", v == "imap", v == "custom", v == "maildoso", v == "mailforge", v == "infraforge":
		return ProviderSMTP
	}
	return ""
}

// securityForPort infers the transport mode from a well-known port.
func securityForPort(port int) string {
	switch port {
	case 465, 993:
		return SecurityTLS
	case 587, 143, 25:
		return SecurityStartTLS
	}
	return ""
}

// endpoint builds an Endpoint, or nil when the vendor gave no host.
func endpoint(host string, port int, username string) *Endpoint {
	host = strings.TrimSpace(host)
	if host == "" {
		return nil
	}
	return &Endpoint{Host: host, Port: port, Security: securityForPort(port), Username: username}
}

// domainOf returns an address's domain, lowercased.
func domainOf(email string) string {
	if i := strings.LastIndexByte(email, '@'); i >= 0 {
		return strings.ToLower(email[i+1:])
	}
	return ""
}

// firstNonEmpty returns the first non-blank value.
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
