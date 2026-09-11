// One parser for the header-style addresses the API hands the UI. They come in
// three shapes: RFC "Name <addr>" (Gmail/Graph sync), "Name (addr)" (the IMAP
// sync, internal/client/smtpimap/imap/address.go), or a bare "addr". Every
// component used to carry its own angle-bracket-only copy, so an IMAP sender
// seeded the reply composer with "Name (addr)" and Send stayed disabled.
const WRAPPED = /[<(]\s*([^<>()\s]+@[^<>()\s]+)\s*[>)]\s*$/;

// The address inside the brackets, or null when there are none.
export function wrappedEmail(s: string): string | null {
    const m = s.match(WRAPPED);
    return m ? m[1] : null;
}

// The bare address: the bracketed one when present, else the trimmed input.
export function bareEmail(s: string): string {
    return wrappedEmail(s) ?? s.trim();
}

// The display name in front of the brackets; the address when there is none.
export function nameFromAddr(s: string): string {
    const m = s.match(WRAPPED);
    if (!m) return s.trim();
    return s.slice(0, m.index).replace(/"/g, "").trim() || m[1];
}
