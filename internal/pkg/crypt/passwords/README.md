# Breached-password denylist

`breached.txt` is the UK National Cyber Security Centre's list of the 100,000
most commonly breached passwords (published from the Have I Been Pwned corpus),
reduced to the 46,528 entries that are 8 to 128 characters long. Anything
shorter is already refused by the length rule, so carrying it here would only
make the file bigger.

Entries are lowercased, deduplicated and sorted. The comparison in
`crypt.CheckPassword` lowercases the candidate, so the casing here is not a
weakening: `Password123` and `password123` are both refused.

Source: https://github.com/danielmiessler/SecLists (Passwords/Common-Credentials/100k-most-used-passwords-NCSC.txt)

Refreshing it is a matter of re-running that filter and committing the result.
Nothing else needs to change.
