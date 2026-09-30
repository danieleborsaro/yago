//go:build !unix

package repo

// windows has no uid to compare, so there only the repo yago was pointed at gets trusted
func lookupOwner(string) (uint32, bool) {
	return 0, false
}
