package service

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBackupRestoreExistingAndAbsent(t *testing.T) {
	root := t.TempDir()
	a, b := filepath.Join(root, "binary"), filepath.Join(root, "absent")
	os.WriteFile(a, []byte("old"), 0755)
	if err := os.Chmod(a, 0755); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "backup")
	if e := save(dir, []string{a, b}, true, true); e != nil {
		t.Fatal(e)
	}
	os.WriteFile(a, []byte("new"), 0700)
	os.WriteFile(b, []byte("created"), 0600)
	m, e := restore(dir, []string{a, b})
	if e != nil || !m.Active {
		t.Fatal(m, e)
	}
	data, _ := os.ReadFile(a)
	if string(data) != "old" {
		t.Fatal(string(data))
	}
	if _, e = os.Stat(b); !os.IsNotExist(e) {
		t.Fatal("absent file not removed")
	}
	fi, _ := os.Stat(a)
	if fi.Mode().Perm() != 0755 {
		t.Fatal(fi.Mode())
	}
}
func TestManifestCannotRestoreArbitraryPaths(t *testing.T) {
	root := t.TempDir()
	a := filepath.Join(root, "a")
	os.WriteFile(a, []byte("old"), 0600)
	dir := filepath.Join(root, "bak")
	save(dir, []string{a}, false, false)
	if _, e := restore(dir, []string{filepath.Join(root, "different")}); e == nil {
		t.Fatal("invalid manifest accepted")
	}
}
func TestAtomicCopyReplacesInsteadOfTruncating(t *testing.T) {
	root := t.TempDir()
	a, b := filepath.Join(root, "a"), filepath.Join(root, "b")
	os.WriteFile(a, []byte("new binary"), 0755)
	os.WriteFile(b, []byte("old binary"), 0755)
	fd, e := os.Open(b)
	if e != nil {
		t.Fatal(e)
	}
	defer fd.Close()
	if e = copyFile(a, b, 0755); e != nil {
		t.Fatal(e)
	}
	old := make([]byte, 10)
	fd.Read(old)
	if string(old) != "old binary" {
		t.Fatal(string(old))
	}
	v, _ := os.ReadFile(b)
	if string(v) != "new binary" {
		t.Fatal(string(v))
	}
}

func TestInstallerLockAndSymlinkGuard(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "install.lock")
	release, e := acquireLock(p)
	if e != nil {
		t.Fatal(e)
	}
	if release2, e := acquireLock(p); e == nil {
		release2()
		t.Fatal("parallel installer allowed")
	}
	release()
	release, e = acquireLock(p)
	if e != nil {
		t.Fatal(e)
	}
	release()
	link := filepath.Join(root, "link")
	os.Symlink(p, link)
	if release, e = acquireLock(link); e == nil {
		release()
		t.Fatal("followed lock symlink")
	}
}
