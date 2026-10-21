package doctor

// ApplyIdentity does nothing on Windows: PUID, PGID, and UMASK are for Linux
// containers, and the server runs as the user who started it.
func ApplyIdentity(config, recordings string) error {
	return nil
}
