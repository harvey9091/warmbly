package imap

// Folder names by role, lowercase, leaf only (the hierarchy prefix is
// stripped before matching). RFC 6154 special-use attributes are always
// preferred; these are the fallback for the servers that attach none, where
// the name is all there is. Older Exchange, cPanel and plenty of hosted
// Dovecots are in that group, and their folders carry the mailbox owner's
// language, so a list of English names alone files a customer's sent mail
// into their inbox.
//
// Kept as exported slices because the spam guard and the Sent resolver read
// them too. Everything here is compared case-insensitively.
var (
	// ImapSpam is also the guard on the warmup "remove from spam" action, so
	// a name that is merely spam-ish (a user folder called "Spam reports")
	// must not appear here.
	ImapSpam = []string{
		"spam", "junk", "junk e-mail", "junk email", "bulk mail",
		"correo no deseado", "no deseado", // Spanish
		"courrier indésirable", "indésirables", "pourriel", // French
		"junk-e-mail", "spamverdacht", // German, Dutch
		"lixo eletrônico", "lixo electrónico", // Portuguese
		"posta indesiderata",                        // Italian
		"skräppost", "uønsket e-post", "roskaposti", // Swedish, Norwegian, Finnish
		"spam-mappe", "levélszemét", // Danish-ish, Hungarian
		"niechciane", "spam-post", // Polish
	}
	ImapSent = []string{
		"sent", "sent mail", "sent items", "sent messages",
		"enviados", "elementos enviados", "correo enviado", // Spanish
		"éléments envoyés", "messages envoyés", "envoyés", // French
		"gesendet", "gesendete elemente", "gesendete objekte", // German
		"verzonden", "verzonden items", // Dutch
		"itens enviados", "enviadas", // Portuguese
		"posta inviata", "inviata", // Italian
		"skickat", "skickade objekt", "sendt", "lähetetyt", // Nordic
		"elküldött elemek", "elküldött üzenetek", // Hungarian
		"elementy wysłane", "wysłane", // Polish
		"odeslaná pošta", "trimise", // Czech, Romanian
	}
	ImapDrafts = []string{
		"draft", "drafts",
		"borradores", "brouillons", "entwürfe", "concepten",
		"rascunhos", "bozze", "utkast", "luonnokset",
		"piszkozatok", "kopie robocze", "koncepty", "ciorne",
	}
	ImapTrash = []string{
		"trash", "bin", "deleted", "deleted items", "deleted messages",
		"papelera", "elementos eliminados", // Spanish
		"corbeille", "éléments supprimés", // French
		"papierkorb", "gelöschte elemente", "gelöschte objekte", // German
		"prullenbak", "verwijderde items", // Dutch
		"lixeira", "itens excluídos", // Portuguese
		"cestino", "posta eliminata", // Italian
		"papperskorgen", "slettet post", "roskakori", // Nordic
		"törölt elemek", "kuka", // Hungarian
		"kosz", "elementy usunięte", // Polish
		"koš", "coș de gunoi", // Czech, Romanian
	}
	ImapArchive = []string{
		"archive", "archives", "all mail",
		"archivo", "archivado", "archives", "archiv",
		"archief", "arquivo", "archivio", "arkiv",
		"arkisto", "archívum", "archiwum", "arhiva",
	}
)
