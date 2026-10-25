package service

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestFreshInstallResetFailureIsAdvisory(t *testing.T) {
	var calls []string
	e := startService(func(a ...string) error {
		calls = append(calls, strings.Join(a, " "))
		if a[0] == "reset-failed" {
			return errors.New("unit not loaded")
		}
		return nil
	})
	want := []string{"daemon-reload", "reset-failed scyllascope", "enable scyllascope", "restart scyllascope"}
	if e != nil || !reflect.DeepEqual(calls, want) {
		t.Fatal(e, calls)
	}
}
func TestStartFailuresOtherThanResetAreFatal(t *testing.T) {
	for _, fail := range []string{"daemon-reload", "enable", "restart"} {
		t.Run(fail, func(t *testing.T) {
			sentinel := errors.New("fail")
			e := startService(func(a ...string) error {
				if a[0] == fail {
					return sentinel
				}
				return nil
			})
			if !errors.Is(e, sentinel) {
				t.Fatal("fatal error swallowed")
			}
		})
	}
}
func TestCorruptBackupValidatedBeforeWrites(t *testing.T) {
	root := t.TempDir()
	a, b := filepath.Join(root, "a"), filepath.Join(root, "b")
	os.WriteFile(a, []byte("old-a"), 0600)
	os.WriteFile(b, []byte("old-b"), 0600)
	dir := filepath.Join(root, "backup")
	if e := save(dir, []string{a, b}, false, false); e != nil {
		t.Fatal(e)
	}
	os.WriteFile(a, []byte("new-a"), 0600)
	os.Remove(filepath.Join(dir, "1"))
	if _, e := restore(dir, []string{a, b}); e == nil {
		t.Fatal("corrupt backup accepted")
	}
	data, _ := os.ReadFile(a)
	if string(data) != "new-a" {
		t.Fatal("partial restore before validation")
	}
}
func TestBackupRejectsSymlinkEntry(t *testing.T) {
	root := t.TempDir()
	a := filepath.Join(root, "a")
	os.WriteFile(a, []byte("old"), 0600)
	dir := filepath.Join(root, "backup")
	save(dir, []string{a}, false, false)
	os.Remove(filepath.Join(dir, "0"))
	os.Symlink(a, filepath.Join(dir, "0"))
	if _, e := validateBackup(dir, []string{a}); e == nil {
		t.Fatal("backup symlink accepted")
	}
}
func TestSoftBudgetOverrideOrder(t *testing.T) {
	a := strings.Index(unit, "Environment=GOMEMLIMIT=128MiB")
	b := strings.Index(unit, "EnvironmentFile=")
	if a < 0 || b <= a || strings.Contains(unit, "MemoryMax=") {
		t.Fatal("soft budget not overridable or hard kill introduced")
	}
}

func TestBackupByteCorruptionRejectedBeforeAnyRestore(t *testing.T) {
	for _, value := range []string{"x", "OLD-B"} {
		t.Run(value, func(t *testing.T) {
			root := t.TempDir()
			a, b := filepath.Join(root, "a"), filepath.Join(root, "b")
			if e := os.WriteFile(a, []byte("old-a"), 0600); e != nil {
				t.Fatal(e)
			}
			if e := os.WriteFile(b, []byte("old-b"), 0600); e != nil {
				t.Fatal(e)
			}
			dir := filepath.Join(root, "backup")
			if e := save(dir, []string{a, b}, false, false); e != nil {
				t.Fatal(e)
			}
			if e := os.WriteFile(a, []byte("new-a"), 0600); e != nil {
				t.Fatal(e)
			}
			if e := os.WriteFile(b, []byte("new-b"), 0600); e != nil {
				t.Fatal(e)
			}
			if e := os.WriteFile(filepath.Join(dir, "1"), []byte(value), 0600); e != nil {
				t.Fatal(e)
			}
			if _, e := restore(dir, []string{a, b}); e == nil {
				t.Fatal("byte-corrupt backup accepted for restoration")
			}
			for _, p := range []string{a, b} {
				got, err := os.ReadFile(p)
				if err != nil {
					t.Fatal(err)
				}
				if string(got) != "new-"+filepath.Base(p) {
					t.Fatal("modified live file before complete backup validation", p)
				}
			}
		})
	}
}

func TestBackupManifestIntegrityModes(t *testing.T) {
	for _, scenario := range []string{"roundtrip", "legacy", "missing_digest", "unknown_version"} {
		t.Run(scenario, func(t *testing.T) {
			root := t.TempDir()
			file := filepath.Join(root, "empty")
			if err := os.WriteFile(file, nil, 0600); err != nil {
				t.Fatal(err)
			}
			dir := filepath.Join(root, "backup")
			if err := save(dir, []string{file}, false, false); err != nil {
				t.Fatal(err)
			}
			manifestFile := filepath.Join(dir, "manifest.json")
			raw, err := os.ReadFile(manifestFile)
			if err != nil {
				t.Fatal(err)
			}
			var m manifest
			if err = json.Unmarshal(raw, &m); err != nil {
				t.Fatal(err)
			}
			if m.FormatVersion != 1 || len(m.Entries[0].SHA256) != 64 || m.Entries[0].Size != 0 {
				t.Fatal("digest metadata missing", m)
			}
			switch scenario {
			case "legacy":
				m.FormatVersion = 0
				m.Entries[0].SHA256 = ""
			case "missing_digest":
				m.Entries[0].SHA256 = ""
			case "unknown_version":
				m.FormatVersion = 99
			}
			raw, err = json.Marshal(m)
			if err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(manifestFile, raw, 0600); err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(file, []byte("new"), 0600); err != nil {
				t.Fatal(err)
			}
			_, err = restore(dir, []string{file})
			valid := scenario == "roundtrip" || scenario == "legacy"
			if (err == nil) != valid {
				t.Fatalf("valid=%t error=%v", valid, err)
			}
			got, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			if valid && len(got) != 0 {
				t.Fatal("empty backup not restored")
			}
			if !valid && string(got) != "new" {
				t.Fatal("invalid manifest modified destination")
			}
		})
	}
}

func TestNewManifestIncludesSecretsAndOldManifestLeavesItUntouched(t *testing.T) {
	root := t.TempDir()
	binary := filepath.Join(root, "binary")
	secret := filepath.Join(root, "secrets.env")
	os.WriteFile(binary, []byte("old binary"), 0600)
	os.WriteFile(secret, []byte("synthetic secret"), 0600)
	backup := filepath.Join(root, "new")
	if err := save(backup, []string{binary, secret}, false, false); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(secret, []byte("changed"), 0600)
	if _, err := restore(backup, []string{binary, secret}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(secret)
	if string(data) != "synthetic secret" {
		t.Fatal("secret not restored")
	}
	old := filepath.Join(root, "old")
	if err := save(old, []string{binary}, false, false); err != nil {
		t.Fatal(err)
	}
	// Production adds this final managed path; validating an older manifest must
	// accept the prefix without synthesizing a destructive absent-file entry.
	m, err := validateBackup(old, []string{binary, "/etc/scyllascope/secrets.env"})
	if err != nil || len(m.Entries) != 1 {
		t.Fatal(m, err)
	}
}

func TestManagedParentsRejectRedirectBeforeAnyWrite(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(root, "scylla-sentinel")
	if err := os.Mkdir(outside, 0700); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(outside, "data")
	if err := os.WriteFile(sentinel, []byte("untouched"), 0600); err != nil {
		t.Fatal(err)
	}
	redirected := filepath.Join(root, "scyllascope")
	if err := os.Symlink(outside, redirected); err != nil {
		t.Fatal(err)
	}
	if err := checkManagedParents([]string{filepath.Join(redirected, "monitor.db")}, filepath.Join(root, "backups")); err == nil {
		t.Fatal("redirected state root accepted")
	}
	info, _ := os.Stat(outside)
	data, _ := os.ReadFile(sentinel)
	if info.Mode().Perm() != 0700 || string(data) != "untouched" {
		t.Fatal("external directory changed")
	}
	if err := checkManagedParents([]string{filepath.Join(root, "missing", "monitor.db")}, filepath.Join(root, "backups")); err != nil {
		t.Fatal("new installation rejected", err)
	}
}
func TestActiveOrEnabledUnmanagedServiceRejected(t *testing.T) {
	for _, state := range []struct {
		active, enabled, present bool
		reject                   bool
	}{{true, false, false, true}, {false, true, false, true}, {true, true, true, false}, {false, false, false, false}} {
		err := validatePriorService(state.active, state.enabled, state.present)
		if (err != nil) != state.reject {
			t.Fatalf("state=%+v error=%v", state, err)
		}
	}
}
