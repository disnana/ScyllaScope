package service

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func startService(run func(...string) error) error {
	if e := run("daemon-reload"); e != nil {
		return e
	}
	// A fresh or unloaded unit has no failure state. Only this advisory action
	// may fail; enable/restart failures remain fatal and trigger rollback.
	_ = run("reset-failed", "scyllascope")
	if e := run("enable", "scyllascope"); e != nil {
		return e
	}
	return run("restart", "scyllascope")
}
func restoreServiceState(m manifest, run func(...string) error) error {
	if e := run("daemon-reload"); e != nil {
		return e
	}
	present := false
	for _, v := range m.Entries {
		if v.Path == managed[1] {
			present = v.Present
		}
	}
	if !present {
		if m.Active || m.Enabled {
			return fmt.Errorf("cannot restore active/enabled state without unit file")
		}
		// Remove dangling enablement symlinks from a failed first install.
		_ = run("disable", "scyllascope")
		return nil
	}
	action := "disable"
	if m.Enabled {
		action = "enable"
	}
	if e := run(action, "scyllascope"); e != nil {
		return e
	}
	if m.Active {
		return run("start", "scyllascope")
	}
	return nil
}
func atomicWrite(path string, data []byte, mode os.FileMode) error {
	f, e := os.CreateTemp(filepath.Dir(path), ".scyllascope-write-*")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	if _, e = f.Write(data); e != nil {
		f.Close()
		return e
	}
	if e = f.Chmod(mode); e != nil {
		f.Close()
		return e
	}
	if e = f.Sync(); e != nil {
		f.Close()
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	if e = os.Rename(f.Name(), path); e != nil {
		return e
	}
	d, e := os.Open(filepath.Dir(path))
	if e != nil {
		return e
	}
	defer d.Close()
	return d.Sync()
}

// Stream hashes rather than loading binary/database/WAL backups into memory.
// This detects corruption, not malicious edits by someone who can change the manifest.
func backupDigest(path string) (int64, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, "", err
	}
	defer f.Close()
	hash := sha256.New()
	size, err := io.Copy(hash, f)
	if err != nil {
		return 0, "", err
	}
	return size, hex.EncodeToString(hash.Sum(nil)), nil
}

// Reject redirected installation roots before chmod/chown, backup or removal.
// Managed default paths are the only paths supported by the native installer.
func checkManagedParents(files []string, backupRoot string) error {
	dirs := map[string]bool{}
	for _, file := range append(append([]string{}, files...), filepath.Join(backupRoot, "manifest.json")) {
		for dir := filepath.Dir(file); dir != "/" && dir != "."; dir = filepath.Dir(dir) {
			dirs[dir] = true
		}
	}
	for dir := range dirs {
		info, err := os.Lstat(dir)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing non-directory or symlinked managed parent %s", dir)
		}
	}
	return nil
}
func validatePriorService(active, enabled, unitPresent bool) error {
	if (active || enabled) && !unitPresent {
		return fmt.Errorf("existing ScyllaScope service is active/enabled without the managed unit file; custom/vendor units require manual migration before installation")
	}
	return nil
}
