# ADA CASA evidence pack

This directory holds the evidence Warmbly submits for the App Defense Alliance
CASA assessment of the Google OAuth client `warmbly-mailboxes`
(project 1010273043313, project id `warmbly-mailboxes`).

| File | What it is |
|---|---|
| `evidence.md` | The submission. One section per CASA test case, with the control, the file and line that implements it, and the artifact that demonstrates it |
| `scope.md` | What is in scope and what is not, and why |
| `crypto-inventory.md` | Every cryptographic operation, its algorithm, key size, key handling and rotation, for test case 4.1.3 |
| `oauth.md` | Every OAuth 2.0 integration, its flow, and the exact Google scopes requested, for 3.2.1 and 3.2.2 |
| `artifacts/` | Scan output and other generated evidence |

## Regenerating the artifacts

    make casa-evidence

That runs the dependency scanners and writes their output under `artifacts/`
with a timestamp. Two artifacts cannot be produced from this repository and
have to be attached by hand before submission:

- the Qualys SSL Labs report for each in-scope hostname (test cases 4.1.1, 4.1.2)
- the authenticated Burp Suite scan using the ADA scan configuration
  (test cases 2.1.1, 2.3.1, 2.3.2, 2.3.4, 3.1.5, 3.1.6, 5.1.1 through 5.1.10, 6.2.1, 6.3.1)

`scope.md` lists the hostnames and the authenticated routes the Burp scan has to
cover.

## What is deliberately not here

The assessment's change log, which records what each control replaced, is kept
outside this repository and given to the lab directly. Warmbly is self-hostable,
so a public account of what a control fixed doubles as a list of what to try
against an instance that has not updated yet. The controls themselves are
described in full above; only the before-and-after is withheld, and only until
deployments have moved on.

## Keeping this current

CASA is annual, and the assurance level can rise. Treat the evidence like the
code it describes: when a control changes, the section that cites it changes in
the same pull request. Every file:line reference here was accurate at the commit
recorded at the top of `evidence.md`.
