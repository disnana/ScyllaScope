// Package service manages only ScyllaScope. It never changes ScyllaDB units,
// databases, firewall rules, or network interfaces.
package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"scyllascope/internal/model"
	"scyllascope/internal/security"
	"scyllascope/internal/settings"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const unit = `[Unit]
Description=ScyllaScope distributed ScyllaDB monitor
After=network-online.target scylla-server.service tailscaled.service
Wants=network-online.target
StartLimitIntervalSec=120
StartLimitBurst=10

[Service]
Type=simple
User=scyllascope
Group=scyllascope
ExecStart=/usr/local/bin/scyllascope
# Soft Go runtime budget, overridable in secrets.env; not an RSS hard cap.
Environment=GOMEMLIMIT=128MiB
EnvironmentFile=-/etc/scyllascope/secrets.env
Restart=on-failure
RestartSec=5
TimeoutStopSec=30
UMask=0077
NoNewPrivileges=true
PrivateTmp=true
ProtectHome=true
ProtectSystem=strict
ReadWritePaths=/var/lib/scyllascope
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true
RestrictSUIDSGID=true
LockPersonality=true
RestrictRealtime=true
RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6 AF_NETLINK
CapabilityBoundingSet=

[Install]
WantedBy=multi-user.target
`

var managed = []string{"/usr/local/bin/scyllascope", "/etc/systemd/system/scyllascope.service", "/etc/scyllascope/config.yaml", "/etc/scyllascope/ui-token", "/var/lib/scyllascope/scyllascope.db", "/var/lib/scyllascope/scyllascope.db-wal", "/var/lib/scyllascope/scyllascope.db-shm", "/etc/scyllascope/secrets.env"}

const backups = "/var/backups/scyllascope"

type backupEntry struct {
	Size    int64  `json:"size,omitempty"`
	SHA256  string `json:"sha256,omitempty"`
	Path    string
	Present bool
	Mode    uint32
	UID     *int `json:"uid,omitempty"`
	GID     *int `json:"gid,omitempty"`
}
type manifest struct {
	FormatVersion int `json:"format_version,omitempty"`
	Time          time.Time
	Active        bool
	Enabled       bool
	Entries       []backupEntry
}

func command(args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "systemctl", args...)
	cmd.WaitDelay = 2 * time.Second
	out, e := cmd.CombinedOutput()
	if e != nil {
		return fmt.Errorf("systemctl %s: %v: %s", strings.Join(args, " "), e, out)
	}
	return nil
}
func requireRoot() error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("requires sudo / root")
	}
	return nil
}
func checkManagedPaths() error {
	for _, p := range managed {
		fi, e := os.Lstat(p)
		if e == nil && !fi.Mode().IsRegular() {
			return fmt.Errorf("refusing non-regular managed path %s", p)
		}
		if e != nil && !os.IsNotExist(e) {
			return e
		}
	}
	return nil
}
func copyFile(src, dst string, mode os.FileMode) error {
	in, e := os.Open(src)
	if e != nil {
		return e
	}
	defer in.Close()
	if e = os.MkdirAll(filepath.Dir(dst), 0750); e != nil {
		return e
	}
	out, e := os.CreateTemp(filepath.Dir(dst), ".scyllascope-copy-*")
	if e != nil {
		return e
	}
	name := out.Name()
	defer os.Remove(name)
	if _, e = io.Copy(out, in); e != nil {
		out.Close()
		return e
	}
	if e = out.Chmod(mode.Perm()); e != nil {
		out.Close()
		return e
	}
	if e = out.Sync(); e != nil {
		out.Close()
		return e
	}
	if e = out.Close(); e != nil {
		return e
	}
	if e = os.Rename(name, dst); e != nil {
		return e
	}
	d, e := os.Open(filepath.Dir(dst))
	if e != nil {
		return e
	}
	defer d.Close()
	return d.Sync()
}

// save/restore are kept separate so the file transaction is tested without systemd.
func save(dir string, paths []string, active, enabled bool) error {
	if e := os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	m := manifest{FormatVersion: 1, Time: time.Now(), Active: active, Enabled: enabled}
	for i, p := range paths {
		entry := backupEntry{Path: p}
		fi, e := os.Lstat(p)
		if e == nil {
			if !fi.Mode().IsRegular() {
				return fmt.Errorf("refusing non-regular backup source %s", p)
			}
			entry.Present = true
			entry.Mode = uint32(fi.Mode().Perm())
			if st, ok := fi.Sys().(*syscall.Stat_t); ok {
				uid, gid := int(st.Uid), int(st.Gid)
				entry.UID = &uid
				entry.GID = &gid
			}
			if e = copyFile(p, filepath.Join(dir, strconv.Itoa(i)), 0600); e != nil {
				return e
			}
			entry.Size, entry.SHA256, e = backupDigest(filepath.Join(dir, strconv.Itoa(i)))
			if e != nil {
				return e
			}
		} else if !os.IsNotExist(e) {
			return e
		}
		m.Entries = append(m.Entries, entry)
	}
	b, e := json.MarshalIndent(m, "", "  ")
	if e != nil {
		return e
	}
	return atomicWrite(filepath.Join(dir, "manifest.json"), b, 0600)
}
func validateBackup(dir string, allowed []string) (manifest, error) {
	var m manifest
	fi, e := os.Lstat(dir)
	if e != nil || !fi.IsDir() {
		return m, fmt.Errorf("invalid backup directory: %v", e)
	}
	fi, e = os.Lstat(filepath.Join(dir, "manifest.json"))
	if e != nil || !fi.Mode().IsRegular() || fi.Size() > 1<<20 {
		return m, fmt.Errorf("invalid backup manifest: %v", e)
	}
	b, e := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if e != nil {
		return m, e
	}
	if e = json.Unmarshal(b, &m); e != nil {
		return m, e
	}
	if m.FormatVersion != 0 && m.FormatVersion != 1 {
		return m, fmt.Errorf("unsupported backup format %d", m.FormatVersion)
	}
	if len(m.Entries) != len(allowed) && !(len(m.Entries) == len(allowed)-1 && allowed[len(allowed)-1] == "/etc/scyllascope/secrets.env") {
		return m, fmt.Errorf("backup manifest mismatch")
	}
	// Validate the entire manifest before modifying anything.
	for i, v := range m.Entries {
		if v.Path != allowed[i] {
			return m, fmt.Errorf("backup path mismatch")
		}
		if v.Present {
			fi, e := os.Lstat(filepath.Join(dir, strconv.Itoa(i)))
			if e != nil {
				return m, e
			}
			if !fi.Mode().IsRegular() {
				return m, fmt.Errorf("non-regular backup entry %d", i)
			}
			if m.FormatVersion == 1 {
				digest, err := hex.DecodeString(v.SHA256)
				if err != nil || len(digest) != sha256.Size || v.Size < 0 || fi.Size() != v.Size {
					return m, fmt.Errorf("backup entry %d size/digest metadata mismatch", i)
				}
				size, hash, err := backupDigest(filepath.Join(dir, strconv.Itoa(i)))
				if err != nil {
					return m, err
				}
				if size != v.Size || hash != v.SHA256 {
					return m, fmt.Errorf("backup entry %d checksum mismatch", i)
				}
			}
		}
	}
	return m, nil
}
func restore(dir string, allowed []string) (manifest, error) {
	m, e := validateBackup(dir, allowed)
	if e != nil {
		return m, e
	}
	if m.FormatVersion == 0 {
		fmt.Fprintln(os.Stderr, "WARNING: legacy backup has no content checksums; file-content integrity cannot be verified.")
	}
	for i, v := range m.Entries {
		if v.Present {
			e = copyFile(filepath.Join(dir, strconv.Itoa(i)), v.Path, os.FileMode(v.Mode))
			if e == nil && v.UID != nil && v.GID != nil {
				e = os.Chown(v.Path, *v.UID, *v.GID)
			}
		} else {
			e = os.Remove(v.Path)
			if os.IsNotExist(e) {
				e = nil
			}
		}
		if e != nil {
			return m, e
		}
	}
	return m, nil
}
func permissions(uid, gid int) error {
	for _, dir := range []string{"/etc/scyllascope", "/var/lib/scyllascope"} {
		if e := os.MkdirAll(dir, 0750); e != nil {
			return e
		}
		if e := os.Chown(dir, 0, gid); e != nil {
			return e
		}
		if e := os.Chmod(dir, 0750); e != nil {
			return e
		}
	}
	if e := os.Chown("/var/lib/scyllascope", uid, gid); e != nil {
		return e
	}
	if e := filepath.WalkDir("/var/lib/scyllascope", func(p string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink in state directory: %s", p)
		}
		return os.Chown(p, uid, gid)
	}); e != nil {
		return e
	}
	for _, p := range []string{"/etc/scyllascope/config.yaml", "/etc/scyllascope/ui-token", "/etc/scyllascope/secrets.env"} {
		if _, e := os.Stat(p); os.IsNotExist(e) {
			continue
		}
		if e := os.Chown(p, 0, gid); e != nil {
			return e
		}
		if e := os.Chmod(p, 0640); e != nil {
			return e
		}
	}
	return nil
}
func acquireLock(path string) (func(), error) {
	fd, e := syscall.Open(path, syscall.O_RDWR|syscall.O_CREAT|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0600)
	if e != nil {
		return nil, e
	}
	f := os.NewFile(uintptr(fd), path)
	if e = syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB); e != nil {
		f.Close()
		return nil, fmt.Errorf("another ScyllaScope install/uninstall/rollback is running")
	}
	return func() { _ = syscall.Flock(fd, syscall.LOCK_UN); _ = f.Close() }, nil
}
func Install(sourceConfig string, c *settings.Config) (retErr error) {
	if e := requireRoot(); e != nil {
		return e
	}
	unlock, lockErr := acquireLock("/run/scyllascope-install.lock")
	if lockErr != nil {
		return lockErr
	}
	defer unlock()
	if e := checkManagedPaths(); e != nil {
		return e
	}
	if c.Security.UITokenFile != "/etc/scyllascope/ui-token" {
		return fmt.Errorf("service installer supports the default ui_token_file only; review a custom systemd unit for %q", c.Security.UITokenFile)
	}
	if c.Storage.SQLitePath != "/var/lib/scyllascope/scyllascope.db" {
		return fmt.Errorf("service installer currently supports the default state path only; use a manually reviewed systemd unit for %q", c.Storage.SQLitePath)
	}
	// Stage the config bytes before stopping anything, including when input=destination.
	var configBytes []byte
	if sourceConfig != "" {
		var e error
		configBytes, e = os.ReadFile(sourceConfig)
		if e != nil && !os.IsNotExist(e) {
			return e
		}
	}
	me, e := os.Executable()
	if e != nil {
		return e
	}
	me, e = filepath.EvalSymlinks(me)
	if e != nil {
		return e
	}
	// Stage binary first: a failed copy must never stop the old service.
	if e = copyFile(me, "/usr/local/bin/.scyllascope-staged", 0755); e != nil {
		return e
	}
	defer os.Remove("/usr/local/bin/.scyllascope-staged")
	account, e := user.Lookup("scyllascope")
	if e != nil {
		out, e := exec.Command("useradd", "--system", "--user-group", "--home-dir", "/var/lib/scyllascope", "--no-create-home", "--shell", "/usr/sbin/nologin", "scyllascope").CombinedOutput()
		if e != nil {
			return fmt.Errorf("create service user: %v %s", e, out)
		}
		account, e = user.Lookup("scyllascope")
		if e != nil {
			return e
		}
	}
	uid, _ := strconv.Atoi(account.Uid)
	gid, _ := strconv.Atoi(account.Gid)
	// Fail visibly rather than run as root or silently skip the real Scylla config.
	if _, e = os.Stat(c.Scylla.ConfigPath); e == nil {
		if e = exec.Command("runuser", "-u", "scyllascope", "--", "test", "-r", c.Scylla.ConfigPath).Run(); e != nil {
			return fmt.Errorf("scyllascope user cannot read %s; grant read-only access to that file and its parent directory, then retry; no DB permissions were changed", c.Scylla.ConfigPath)
		}
	}
	active := exec.Command("systemctl", "is-active", "--quiet", "scyllascope").Run() == nil
	enabled := exec.Command("systemctl", "is-enabled", "--quiet", "scyllascope").Run() == nil
	if _, e = os.Stat(managed[1]); e == nil {
		if e = command("stop", "scyllascope"); e != nil {
			return e
		}
	}
	dir := filepath.Join(backups, time.Now().UTC().Format("20060102T150405.000000000Z"))
	if e = save(dir, managed, active, enabled); e != nil {
		if active {
			_ = command("start", "scyllascope")
		}
		return e
	}
	installed := false
	defer func() {
		if !installed {
			// Never replace database/WAL files while the new process might be alive.
			if _, statErr := os.Stat(managed[1]); statErr == nil {
				if stopErr := command("stop", "scyllascope"); stopErr != nil {
					retErr = errors.Join(retErr, fmt.Errorf("rollback aborted: could not stop service; backup %s: %w", dir, stopErr))
					return
				}
			}
			m, restoreErr := restore(dir, managed)
			if restoreErr == nil {
				restoreErr = restoreServiceState(m, command)
			}
			if restoreErr != nil {
				retErr = errors.Join(retErr, fmt.Errorf("rollback incomplete; backup %s: %w", dir, restoreErr))
			} else {
				fmt.Fprintf(os.Stderr, "Installation failed; restored backup %s. Verify service status.\n", dir)
			}
		}
	}()
	if e = os.MkdirAll("/etc/scyllascope", 0750); e != nil {
		return e
	}
	if configBytes != nil {
		if e = atomicWrite(managed[2], configBytes, 0640); e != nil {
			return e
		}
	} else if _, e = os.Stat(managed[2]); os.IsNotExist(e) {
		if e = atomicWrite(managed[2], []byte("# ScyllaScope overrides. Scylla settings are detected at every start.\n"), 0640); e != nil {
			return e
		}
	}
	if _, e = os.Stat(managed[3]); os.IsNotExist(e) {
		token, e := security.Token()
		if e != nil {
			return e
		}
		if e = atomicWrite(managed[3], []byte(token+"\n"), 0640); e != nil {
			return e
		}
	}
	if e = permissions(uid, gid); e != nil {
		return e
	}
	if e = os.Rename("/usr/local/bin/.scyllascope-staged", managed[0]); e != nil {
		return e
	}
	if e = atomicWrite(managed[1], []byte(unit), 0644); e != nil {
		return e
	}
	if e = startService(command); e != nil {
		return e
	}
	// Process liveness only; a DB or peer outage is not an install failure.
	if e = waitHealthy(c, 20*time.Second); e != nil {
		_ = command("stop", "scyllascope")
		return e
	}
	if e = atomicWrite(filepath.Join(backups, "latest"), []byte(filepath.Base(dir)), 0600); e != nil {
		return fmt.Errorf("record rollback pointer: %w", e)
	}
	installed = true
	fmt.Printf("Installed ScyllaScope as unprivileged user scyllascope.\nBackup: %s\nUI: %s://%s\nLogin token: sudo cat /etc/scyllascope/ui-token\n", dir, scheme(c), c.Listen)
	if c.Token == "" {
		fmt.Println("Peer sharing is disabled until the same cluster_token is configured on all observers. Scylla topology discovery still works locally.")
	}
	return nil
}
func scheme(c *settings.Config) string {
	if c.Security.TLSCert != "" {
		return "https"
	}
	return "http"
}
func waitHealthy(c *settings.Config, timeout time.Duration) error {
	client := &http.Client{Timeout: time.Second, Transport: &http.Transport{Proxy: nil}}
	defer client.CloseIdleConnections()
	end := time.Now().Add(timeout)
	stable := 0
	for time.Now().Before(end) {
		if exec.Command("systemctl", "is-active", "--quiet", "scyllascope").Run() == nil {
			stable++
			if c.Security.TLSCert != "" && stable >= 6 {
				return nil
			} // TLS identity is checked by client; don't skip verification here.
			if c.Security.TLSCert == "" {
				r, e := client.Get("http://" + c.Listen + "/healthz")
				if e == nil {
					var health struct {
						OK      bool   `json:"ok"`
						Version string `json:"version"`
						Service string `json:"service"`
					}
					err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&health)
					r.Body.Close()
					if r.StatusCode == 200 && err == nil && health.OK && health.Version == model.Version && health.Service == "scyllascope" {
						return nil
					}
				}
			}
		} else {
			stable = 0
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("ScyllaScope did not become live; inspect journalctl -u scyllascope (automatic rollback follows)")
}
func Rollback() error {
	if e := requireRoot(); e != nil {
		return e
	}
	unlock, lockErr := acquireLock("/run/scyllascope-install.lock")
	if lockErr != nil {
		return lockErr
	}
	defer unlock()
	if e := checkManagedPaths(); e != nil {
		return e
	}
	b, e := os.ReadFile(filepath.Join(backups, "latest"))
	if e != nil {
		return e
	}
	name := strings.TrimSpace(string(b))
	if name == "" || filepath.Base(name) != name || name == "." || name == ".." {
		return fmt.Errorf("invalid backup pointer")
	}
	dir := filepath.Join(backups, name)
	if _, e = validateBackup(dir, managed); e != nil {
		return e
	}
	if e = command("stop", "scyllascope"); e != nil {
		return e
	}
	m, e := restore(dir, managed)
	if e != nil {
		return e
	}
	if e = restoreServiceState(m, command); e != nil {
		return e
	}
	fmt.Printf("Restored %s (including saved config and monitor history; Scylla DB untouched).\n", dir)
	return e
}
func Uninstall(purge bool) error {
	if e := requireRoot(); e != nil {
		return e
	}
	unlock, lockErr := acquireLock("/run/scyllascope-install.lock")
	if lockErr != nil {
		return lockErr
	}
	defer unlock()
	if e := command("disable", "--now", "scyllascope"); e != nil {
		return e
	}
	for _, p := range []string{managed[0], managed[1]} {
		if e := os.Remove(p); e != nil && !os.IsNotExist(e) {
			return e
		}
	}
	if e := command("daemon-reload"); e != nil {
		return e
	}
	if purge {
		for _, p := range []string{"/etc/scyllascope", "/var/lib/scyllascope"} {
			if e := os.RemoveAll(p); e != nil {
				return e
			}
		}
	}
	fmt.Println("Service removed. Backups under /var/backups/scyllascope were retained; they may contain credentials.")
	return nil
}
func Status() error  { return interactive("systemctl", "--no-pager", "--full", "status", "scyllascope") }
func Restart() error { return interactive("systemctl", "restart", "scyllascope") }
func Logs() error    { return interactive("journalctl", "-u", "scyllascope", "-f", "--no-pager") }
func interactive(binary string, args ...string) error {
	cmd := exec.Command(binary, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
