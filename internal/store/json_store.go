package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"dx-util-env-manager/internal/config"
	cryptoutil "dx-util-env-manager/internal/crypto"
)

type JSONStore struct {
	path        string
	profile     string
	allowReinit bool
	warnW       io.Writer
}

func NewJSONStore(path, profile string, allowReinit bool, warnW io.Writer) *JSONStore {
	return &JSONStore{path: path, profile: profile, allowReinit: allowReinit, warnW: warnW}
}

// --- public interface ---

func (s *JSONStore) EnvForDir(dir string) (map[string]string, error) {
	db, key, err := s.loadAndMigrate()
	if err != nil {
		return nil, err
	}
	id := cryptoutil.FolderID(dir, key)
	folder := db.Folders[id]
	if folder == nil {
		return map[string]string{}, nil
	}
	return cloneEnv(folder[s.profile]), nil
}

func (s *JSONStore) SetEnvForDir(dir string, env map[string]string) error {
	db, key, err := s.loadAndMigrate()
	if err != nil {
		return err
	}
	s.ensureFolders(&db)
	id := cryptoutil.FolderID(dir, key)
	if db.Folders[id] == nil {
		db.Folders[id] = map[string]map[string]string{}
	}
	db.Folders[id][s.profile] = cloneEnv(env)
	return s.saveDB(db, key)
}

func (s *JSONStore) GetVar(dir, k string) (string, bool, error) {
	db, key, err := s.loadAndMigrate()
	if err != nil {
		return "", false, err
	}
	id := cryptoutil.FolderID(dir, key)
	folder := db.Folders[id]
	if folder == nil {
		return "", false, nil
	}
	vars := folder[s.profile]
	if vars == nil {
		return "", false, nil
	}
	v, ok := vars[k]
	return v, ok, nil
}

func (s *JSONStore) SetVar(dir, k, v string) error {
	db, key, err := s.loadAndMigrate()
	if err != nil {
		return err
	}
	s.ensureFolders(&db)
	id := cryptoutil.FolderID(dir, key)
	if db.Folders[id] == nil {
		db.Folders[id] = map[string]map[string]string{}
	}
	if db.Folders[id][s.profile] == nil {
		db.Folders[id][s.profile] = map[string]string{}
	}
	db.Folders[id][s.profile][k] = v
	return s.saveDB(db, key)
}

func (s *JSONStore) UnsetVar(dir, k string) error {
	db, key, err := s.loadAndMigrate()
	if err != nil {
		return err
	}
	id := cryptoutil.FolderID(dir, key)
	folder := db.Folders[id]
	if folder == nil {
		return nil
	}
	vars := folder[s.profile]
	if vars == nil {
		return nil
	}
	delete(vars, k)
	return s.saveDB(db, key)
}

func (s *JSONStore) ListVars(dir string) (map[string]string, error) {
	return s.EnvForDir(dir)
}

func (s *JSONStore) Rotate(oldKey, newKey []byte) error {
	db, err := s.loadDBWithKey(oldKey)
	if err != nil {
		return fmt.Errorf("load with old key: %w", err)
	}

	rehashed := make(map[string]map[string]map[string]string, len(db.Folders))
	for oldID, profiles := range db.Folders {
		newID := cryptoutil.FolderID(oldID, newKey)
		rehashed[newID] = profiles
	}
	db.Folders = rehashed
	db.Version = CurrentVersion

	if err := s.saveDB(db, newKey); err != nil {
		return fmt.Errorf("save with new key: %w", err)
	}
	return cryptoutil.SetKey(newKey)
}

func (s *JSONStore) Doctor() DoctorResult {
	res := DoctorResult{OK: true}

	key, err := cryptoutil.GetKey()
	if err != nil {
		res.OK = false
		res.Errors = append(res.Errors, "keychain: "+err.Error())
		return res
	}

	db, err := s.loadDBWithKey(key)
	if err != nil {
		res.OK = false
		res.Errors = append(res.Errors, "store decrypt: "+err.Error())
		return res
	}

	res.FolderCount = len(db.Folders)
	for id, profiles := range db.Folders {
		if len(id) != 64 {
			res.Errors = append(res.Errors, fmt.Sprintf("folder %q: unexpected ID length", id))
			res.OK = false
		}
		for _, vars := range profiles {
			res.TotalVars += len(vars)
		}
	}
	return res
}

// --- internals ---

func (s *JSONStore) ensureFolders(db *DB) {
	if db.Folders == nil {
		db.Folders = map[string]map[string]map[string]string{}
	}
}

// loadAndMigrate loads the DB, migrating v2→v3 transparently.
func (s *JSONStore) loadAndMigrate() (DB, []byte, error) {
	// Probe keychain to distinguish post-purge (absent) from mismatch (present).
	_, probeErr := cryptoutil.GetKey()
	keyWasAbsent := errors.Is(probeErr, cryptoutil.ErrKeyNotFound)

	key, err := cryptoutil.GetOrGenerateKey()
	if err != nil {
		return DB{}, nil, fmt.Errorf("keyring: %w", err)
	}

	db, err := s.loadDBWithKey(key)
	if err != nil {
		// Guarded re-init: opt-in flag + no prior key + decryption integrity failure.
		if s.allowReinit && keyWasAbsent && strings.Contains(err.Error(), "decrypt store") {
			os.Remove(s.path)
			fmt.Fprintln(s.warnW, "warning: store was unreadable (no master key); purged and re-initialized a fresh store.")
			return DB{Version: CurrentVersion, Folders: map[string]map[string]map[string]string{}}, key, nil
		}
		return DB{}, nil, err
	}

	return db, key, nil
}

// dbV2 is the legacy v2 store shape for migration.
type dbV2 struct {
	Version int                          `json:"version"`
	Folders map[string]map[string]string `json:"folders"`
}

func (s *JSONStore) loadDBWithKey(key []byte) (DB, error) {
	db := DB{
		Version: CurrentVersion,
		Folders: map[string]map[string]map[string]string{},
	}

	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return db, nil
		}
		return db, err
	}
	if len(data) == 0 {
		return db, nil
	}

	plaintext, err := cryptoutil.Decrypt(data, key)
	if err != nil {
		return db, fmt.Errorf("decrypt store: %w", err)
	}

	// Peek at version to determine parsing strategy.
	var ver struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(plaintext, &ver); err != nil {
		return db, fmt.Errorf("decode store: %w", err)
	}

	if ver.Version < 2 {
		return db, fmt.Errorf("store version %d is no longer supported; this store must be migrated with de v1.0.0 before upgrading", ver.Version)
	}

	if ver.Version == 2 {
		// v2: folders are map[string]map[string]string — migrate to v3 in memory.
		var old dbV2
		if err := json.Unmarshal(plaintext, &old); err != nil {
			return db, fmt.Errorf("decode store: %w", err)
		}
		for id, vars := range old.Folders {
			db.Folders[id] = map[string]map[string]string{config.DefaultProfile: vars}
		}
		// Persist the migration atomically.
		db.Version = CurrentVersion
		if err := s.saveDB(db, key); err != nil {
			return db, fmt.Errorf("migrate v2→v3: %w", err)
		}
		return db, nil
	}

	// v3+
	if err := json.Unmarshal(plaintext, &db); err != nil {
		return db, fmt.Errorf("decode store: %w", err)
	}
	if db.Folders == nil {
		db.Folders = map[string]map[string]map[string]string{}
	}
	return db, nil
}

func (s *JSONStore) saveDB(db DB, key []byte) error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(db, "", "  ")
	if err != nil {
		return err
	}

	ciphertext, err := cryptoutil.Encrypt(data, key)
	if err != nil {
		return fmt.Errorf("encrypt store: %w", err)
	}

	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, ciphertext, 0600); err != nil {
		return err
	}

	// Atomic replace: remove then rename (Windows-safe).
	if err := os.Remove(s.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.Rename(tmp, s.path)
}
