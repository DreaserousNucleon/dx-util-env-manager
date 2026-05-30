package store

const CurrentVersion = 3

// DB is the decrypted store shape (v3).
// Folders maps folderID → profile → vars.
type DB struct {
	Version int                                     `json:"version"`
	Folders map[string]map[string]map[string]string `json:"folders"`
}

// DoctorResult holds the output of a store integrity check.
type DoctorResult struct {
	OK          bool
	FolderCount int
	TotalVars   int
	Errors      []string
}

type EnvStore interface {
	EnvForDir(dir string) (map[string]string, error)
	SetEnvForDir(dir string, env map[string]string) error
	GetVar(dir, key string) (string, bool, error)
	SetVar(dir, key, value string) error
	UnsetVar(dir, key string) error
	ListVars(dir string) (map[string]string, error)
	// Rotate re-encrypts the store with newKey and updates the keychain.
	Rotate(oldKey, newKey []byte) error
	// Doctor checks store integrity.
	Doctor() DoctorResult
}

func cloneEnv(src map[string]string) map[string]string {
	if src == nil {
		return map[string]string{}
	}

	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}

	return dst
}
