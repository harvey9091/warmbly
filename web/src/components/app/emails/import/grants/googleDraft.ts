// The Google domain proof's draft, held by GrantImportWizard so moving between steps loses nothing.
import React from "react";
import type { GoogleGrantStart } from "@/lib/api/models/app/emails/MailboxSources";

export interface GrantError {
    text: string;
    code?: string;
}

/** What the Google proof step holds while someone moves between steps. */
export interface GoogleDraft {
    domain: string;
    setDomain: (v: string) => void;
    admin: string;
    setAdmin: (v: string) => void;
    tried: boolean;
    setTried: (v: boolean) => void;
    /** The proof belongs to the domain and admin it was asked for. */
    proof: (GoogleGrantStart & { domain: string; admin: string }) | null;
    setProof: (p: GoogleDraft["proof"]) => void;
    dnsOpen: boolean;
    setDnsOpen: (v: boolean) => void;
    error: GrantError | null;
    setError: (e: GrantError | null) => void;
    dirty: boolean;
    reset: () => void;
}

export function useGoogleDraft(initialDomain = ""): GoogleDraft {
    const [domain, setDomain] = React.useState(initialDomain);
    const [admin, setAdmin] = React.useState("");
    const [tried, setTried] = React.useState(false);
    const [proof, setProof] = React.useState<GoogleDraft["proof"]>(null);
    const [dnsOpen, setDnsOpen] = React.useState(false);
    const [error, setError] = React.useState<GrantError | null>(null);
    const reset = React.useCallback(() => {
        setDomain("");
        setAdmin("");
        setTried(false);
        setProof(null);
        setDnsOpen(false);
        setError(null);
    }, []);
    const dirty = domain.trim() !== initialDomain.trim() || admin.trim() !== "";
    return { domain, setDomain, admin, setAdmin, tried, setTried, proof, setProof, dnsOpen, setDnsOpen, error, setError, dirty, reset };
}
