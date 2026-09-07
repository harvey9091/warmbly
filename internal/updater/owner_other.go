//go:build !unix

package updater

func ownerOf(string) (uid, gid int, ok bool) { return 0, 0, false }
