//! Source-network classification for the tracking endpoints.
//!
//! Security gateways fetch the pixel and walk every link in a message with an
//! ordinary browser user agent, which is exactly what the UA rules in
//! `abuse.rs` cannot see. What gives them away is where they come from: a
//! mail-filtering network, never a person's own device.
//!
//! A match never changes the response. The pixel is still served and the click
//! is still redirected, because a scanner that is refused is a scanner that
//! reports the link as dead. Only the published event is labelled, and the
//! consumer records it as a machine open or click: kept as delivery evidence,
//! never counted as engagement.
//!
//! Two scopes, because the doubt is not symmetric. A network that only ever
//! filters mail (`Scope::All`) is a machine whatever it asks for. A network
//! that also carries a mail client's own image fetches (`Scope::Clicks`) can
//! only be judged on click tickets: Microsoft and Google both proxy external
//! images for their web mail, so treating their pixel fetches as machines
//! would zero the open rate for every recipient on them, while a person's
//! click is always their own browser talking to us directly.

use axum::http::HeaderMap;
use ipnet::IpNet;
use std::collections::HashMap;
use std::net::IpAddr;
use std::sync::Arc;

/// The catalogue shipped with the service. Operators extend it with
/// TRACKING_SCANNER_NETWORKS and TRACKING_SCANNER_CLICK_NETWORKS.
const BUILTIN_CATALOGUE: &str = include_str!("../scanner-networks.txt");

/// What a source may be judged on.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum Scope {
    /// Every request from the source is automated.
    All,
    /// Only click tickets. The source also carries a mail client's own image
    /// fetches, which are genuine opens.
    Clicks,
}

impl Scope {
    fn covers(self, kind: Request) -> bool {
        match self {
            Scope::All => true,
            Scope::Clicks => kind == Request::Click,
        }
    }
}

/// Which endpoint is asking.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum Request {
    Open,
    Click,
}

struct Entry {
    scope: Scope,
    label: Arc<str>,
}

/// Matches a request's source against the known-scanner catalogue.
#[derive(Default)]
pub struct ScannerNetworks {
    nets: Vec<(IpNet, Entry)>,
    asns: HashMap<u32, Entry>,
    /// Header a trusted proxy sets with the source ASN. None disables ASN
    /// matching entirely, which is the default: an ASN is not something the
    /// service can work out on its own.
    asn_header: Option<String>,
}

impl ScannerNetworks {
    /// Builds the matcher from the shipped catalogue plus the operator's own
    /// entries. Unparseable entries are skipped with a warning rather than
    /// failing the boot: one typo in an allowlist must not take tracking down.
    pub fn new(
        builtins: bool,
        all_networks: &str,
        click_networks: &str,
        asn_header: Option<String>,
    ) -> Self {
        let mut s = Self {
            asn_header: asn_header.filter(|h| !h.trim().is_empty()).map(|h| {
                let mut h = h.trim().to_ascii_lowercase();
                h.retain(|c| !c.is_whitespace());
                h
            }),
            ..Default::default()
        };
        if builtins {
            s.load_catalogue(BUILTIN_CATALOGUE, None);
        }
        s.load_catalogue(all_networks, Some(Scope::All));
        s.load_catalogue(click_networks, Some(Scope::Clicks));
        tracing::info!(
            "Scanner catalogue: {} networks, {} ASNs (asn header: {})",
            s.nets.len(),
            s.asns.len(),
            s.asn_header.as_deref().unwrap_or("none")
        );
        if !s.asns.is_empty() && s.asn_header.is_none() {
            tracing::warn!("Scanner catalogue has ASN entries but TRACKING_SCANNER_ASN_HEADER is unset, so none of them can match");
        }
        s
    }

    /// Reads `<source> [scope] [label]` lines, `#` to end of line a comment.
    /// `force` overrides the scope column, which is how the two operator
    /// variables get their meaning while sharing one parser with the file.
    fn load_catalogue(&mut self, text: &str, force: Option<Scope>) {
        for raw in text.split(['\n', ',']) {
            let line = raw.split('#').next().unwrap_or("").trim();
            if line.is_empty() {
                continue;
            }
            let mut fields = line.split_whitespace();
            let Some(source) = fields.next() else {
                continue;
            };
            // An entry that names no scope gets the cautious one.
            let mut scope = force.unwrap_or(Scope::Clicks);
            let mut label = None;
            for field in fields {
                match field {
                    "all" | "clicks" if force.is_some() => {}
                    "all" => scope = Scope::All,
                    "clicks" => scope = Scope::Clicks,
                    other if label.is_none() => label = Some(other),
                    _ => {}
                }
            }
            self.insert(source, scope, Arc::from(label.unwrap_or("scanner")));
        }
    }

    fn insert(&mut self, source: &str, scope: Scope, label: Arc<str>) {
        if let Some(num) = source
            .strip_prefix("asn:")
            .or_else(|| source.strip_prefix("AS"))
        {
            match num.trim().parse::<u32>() {
                Ok(asn) => {
                    self.asns.insert(asn, Entry { scope, label });
                }
                Err(_) => tracing::warn!("scanner catalogue: ignoring unparseable ASN {source:?}"),
            }
            return;
        }
        // A bare address is the /32 or /128 containing it.
        let parsed = source
            .parse::<IpNet>()
            .or_else(|_| source.parse::<IpAddr>().map(IpNet::from));
        match parsed {
            Ok(net) => self.nets.push((net.trunc(), Entry { scope, label })),
            Err(_) => tracing::warn!("scanner catalogue: ignoring unparseable network {source:?}"),
        }
    }

    /// The label of the scanner source this request came from, if the source
    /// is known and its scope covers this kind of request.
    ///
    /// `trusted_peer` is whether the socket peer is one of the configured
    /// proxies. The ASN header is read only then, for the same reason the
    /// client address is: a header anyone can set is a header that lets a
    /// caller pick its own classification.
    pub fn classify(
        &self,
        ip: &str,
        headers: &HeaderMap,
        trusted_peer: bool,
        kind: Request,
    ) -> Option<Arc<str>> {
        if let Some(asn) = self.source_asn(headers, trusted_peer) {
            if let Some(entry) = self.asns.get(&asn) {
                if entry.scope.covers(kind) {
                    return Some(entry.label.clone());
                }
            }
        }
        let addr = ip.parse::<IpAddr>().ok()?;
        self.nets
            .iter()
            .find(|(net, entry)| entry.scope.covers(kind) && net.contains(&addr))
            .map(|(_, entry)| entry.label.clone())
    }

    fn source_asn(&self, headers: &HeaderMap, trusted_peer: bool) -> Option<u32> {
        if !trusted_peer {
            return None;
        }
        let name = self.asn_header.as_deref()?;
        let raw = headers.get(name)?.to_str().ok()?.trim();
        // Cloudflare writes the bare number; some edges prefix it with AS.
        raw.strip_prefix("AS")
            .or_else(|| raw.strip_prefix("as"))
            .unwrap_or(raw)
            .parse()
            .ok()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn hdr(pairs: &[(&'static str, &'static str)]) -> HeaderMap {
        let mut h = HeaderMap::new();
        for (k, v) in pairs {
            h.insert(*k, v.parse().unwrap());
        }
        h
    }

    fn builtins() -> ScannerNetworks {
        ScannerNetworks::new(true, "", "", Some("cf-asn".into()))
    }

    // Exchange Online Protection filters mail and reads none, so both the
    // pixel it fetches and the ticket it walks are machines.
    #[test]
    fn exchange_online_protection_is_a_scanner_for_both() {
        let s = builtins();
        for kind in [Request::Open, Request::Click] {
            assert_eq!(
                s.classify("40.107.1.2", &hdr(&[]), false, kind).as_deref(),
                Some("microsoft-365-protection"),
                "{kind:?} from EOP should be labelled"
            );
        }
        assert_eq!(
            s.classify("2a01:111:f400::1", &hdr(&[]), false, Request::Open)
                .as_deref(),
            Some("microsoft-365-protection")
        );
    }

    // The whole of Microsoft's network is a scanner for click tickets and must
    // never be one for pixels: Outlook on the web fetches external images
    // through Microsoft's own proxy, so a genuine open arrives from there.
    #[test]
    fn microsoft_asn_is_clicks_only() {
        let s = builtins();
        let h = hdr(&[("cf-asn", "8075")]);
        assert_eq!(
            s.classify("13.107.128.5", &h, true, Request::Click)
                .as_deref(),
            Some("microsoft")
        );
        assert_eq!(s.classify("13.107.128.5", &h, true, Request::Open), None);
    }

    // Outlook on the web is served from the Exchange Online ranges, which are
    // deliberately absent from the catalogue.
    #[test]
    fn exchange_online_itself_is_not_a_scanner() {
        let s = builtins();
        assert_eq!(
            s.classify("40.100.0.1", &hdr(&[]), false, Request::Open),
            None
        );
        assert_eq!(
            s.classify("52.96.0.1", &hdr(&[]), false, Request::Click),
            None
        );
    }

    // The ASN header is a header, so it is believed only from a proxy the
    // operator named. Otherwise any caller could pick its own label, or ask
    // not to be labelled at all.
    #[test]
    fn asn_header_is_ignored_from_an_untrusted_peer() {
        let s = builtins();
        let h = hdr(&[("cf-asn", "8075")]);
        assert_eq!(s.classify("203.0.113.9", &h, false, Request::Click), None);
        assert_eq!(
            s.classify("203.0.113.9", &h, true, Request::Click)
                .as_deref(),
            Some("microsoft")
        );
    }

    // With no header named, an ASN cannot be established at all and the
    // catalogue's ASN entries are inert.
    #[test]
    fn asn_entries_need_a_configured_header() {
        let s = ScannerNetworks::new(true, "", "", None);
        let h = hdr(&[("cf-asn", "8075")]);
        assert_eq!(s.classify("203.0.113.9", &h, true, Request::Click), None);
    }

    #[test]
    fn operator_entries_take_their_scope_from_the_variable_they_are_in() {
        let s = ScannerNetworks::new(
            false,
            "203.0.113.0/24 proofpoint, 198.51.100.7",
            "192.0.2.0/24 acme-gateway",
            None,
        );
        assert_eq!(
            s.classify("203.0.113.9", &hdr(&[]), false, Request::Open)
                .as_deref(),
            Some("proofpoint")
        );
        // A bare address is its own /32, and needs no label.
        assert_eq!(
            s.classify("198.51.100.7", &hdr(&[]), false, Request::Open)
                .as_deref(),
            Some("scanner")
        );
        assert_eq!(
            s.classify("192.0.2.5", &hdr(&[]), false, Request::Click)
                .as_deref(),
            Some("acme-gateway")
        );
        assert_eq!(
            s.classify("192.0.2.5", &hdr(&[]), false, Request::Open),
            None
        );
    }

    // One bad line must not take the rest of the catalogue with it, and must
    // not stop the service booting.
    #[test]
    fn unparseable_entries_are_skipped() {
        let s = ScannerNetworks::new(false, "not-a-cidr, 203.0.113.0/24 pf, asn:nope", "", None);
        assert_eq!(
            s.classify("203.0.113.9", &hdr(&[]), false, Request::Open)
                .as_deref(),
            Some("pf")
        );
    }

    #[test]
    fn builtins_can_be_turned_off() {
        let s = ScannerNetworks::new(false, "", "", Some("cf-asn".into()));
        assert!(s.nets.is_empty() && s.asns.is_empty());
        assert_eq!(
            s.classify("40.107.1.2", &hdr(&[]), false, Request::Open),
            None
        );
    }

    // The shipped file is parsed at compile time; a typo in it would silently
    // disarm the default the whole feature rests on.
    #[test]
    fn shipped_catalogue_parses_completely() {
        let s = builtins();
        let lines = BUILTIN_CATALOGUE
            .lines()
            .filter(|l| {
                let l = l.split('#').next().unwrap_or("").trim();
                !l.is_empty()
            })
            .count();
        assert_eq!(s.nets.len() + s.asns.len(), lines, "every entry loaded");
    }
}
