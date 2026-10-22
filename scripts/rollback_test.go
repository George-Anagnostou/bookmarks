package scripts

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Exercise the rollback's remote shell body locally with temporary files and
// fake system commands. These tests never connect to a host or control services.
func TestRollbackRemote(t *testing.T) {
	source, err := os.ReadFile("rollback-bookmarkd.sh")
	if err != nil {
		t.Fatal(err)
	}
	_, body, ok := strings.Cut(string(source), "<<'EOF'\n")
	if !ok {
		t.Fatal("remote script start not found")
	}
	body, _, ok = strings.Cut(body, "\nEOF\n")
	if !ok {
		t.Fatal("remote script end not found")
	}

	for _, tt := range []struct {
		name          string
		dbVersion     string // empty: no live database
		recordVersion string // empty: no pre-deploy record
		savedDB       string // pre-deploy.db contents; empty: missing
		noPrevious    bool
		wantPlan      string // mode printed by "plan"; empty: plan fails
		applyMode     string
		wantError     bool
		wantCommands  string
		wantDB        string
	}{
		{
			name: "compatible database restores binary only", dbVersion: "1", recordVersion: "1",
			savedDB: "version=1", wantPlan: "binary", applyMode: "binary",
			wantCommands: "stop bookmarkd\nstart bookmarkd\n--no-pager --full status bookmarkd\n",
			wantDB:       "version=1-live",
		},
		{
			name: "migrated database restores pre-deploy backup", dbVersion: "2", recordVersion: "1",
			savedDB: "version=1", wantPlan: "restore", applyMode: "restore",
			wantCommands: "stop bookmarkd\nstop bookmarks-backup.timer bookmarks-backup.service\n" +
				"start bookmarkd\nstart bookmarks-backup.timer\n--no-pager --full status bookmarkd\n",
			wantDB: "version=1",
		},
		{
			name: "plan changed since confirmation", dbVersion: "2", recordVersion: "1",
			savedDB: "version=1", wantPlan: "restore", applyMode: "binary", wantError: true,
			wantDB: "version=2-live",
		},
		{
			name: "missing record needs explicit binary mode", dbVersion: "1",
			wantPlan: "unknown", applyMode: "binary",
			wantCommands: "stop bookmarkd\nstart bookmarkd\n--no-pager --full status bookmarkd\n",
			wantDB:       "version=1-live",
		},
		{
			name: "missing record never restores", dbVersion: "1",
			wantPlan: "unknown", applyMode: "restore", wantError: true,
			wantDB: "version=1-live",
		},
		{
			name: "corrupt pre-deploy backup is refused", dbVersion: "2", recordVersion: "1",
			savedDB: "version=1 corrupt", applyMode: "restore", wantError: true,
			wantDB: "version=2-live",
		},
		{
			name: "pre-deploy backup with wrong version is refused", dbVersion: "2", recordVersion: "1",
			savedDB: "version=2", applyMode: "restore", wantError: true,
			wantDB: "version=2-live",
		},
		{
			name: "missing previous binary", dbVersion: "1", recordVersion: "1", noPrevious: true,
			applyMode: "binary", wantError: true, wantDB: "version=1-live",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			bin := filepath.Join(dir, "bin")
			backupDir := filepath.Join(dir, "backups")
			for _, d := range []string{bin, backupDir} {
				if err := os.Mkdir(d, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			write := func(path, contents string) {
				t.Helper()
				if err := os.WriteFile(path, []byte(contents), 0o700); err != nil {
					t.Fatal(err)
				}
			}
			read := func(path string) string {
				t.Helper()
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				return string(data)
			}

			db := filepath.Join(dir, "live database.db")
			if tt.dbVersion != "" {
				write(db, "version="+tt.dbVersion+"-live")
				write(db+"-wal", "wal")
			}
			if tt.recordVersion != "" {
				write(filepath.Join(backupDir, "pre-deploy.env"),
					"PRE_DEPLOY_SCHEMA_VERSION="+tt.recordVersion+"\nPRE_DEPLOY_BACKUP=/backups/bookmarks-x.db\n")
			}
			if tt.savedDB != "" {
				write(filepath.Join(backupDir, "pre-deploy.db"), tt.savedDB)
			}
			config := filepath.Join(dir, "bookmarkd.env")
			write(config, "BOOKMARKS_DBPATH='"+db+"'\nBOOKMARKS_BACKUP_DIR='"+backupDir+"'\n")

			write(filepath.Join(bin, "id"), "#!/bin/sh\necho 0\n")
			write(filepath.Join(bin, "install"), "#!/bin/sh\nexec /bin/cp \"$7\" \"$8\"\n")
			write(filepath.Join(bin, "systemctl"), "#!/bin/sh\necho \"$*\" >> \"$TEST_COMMAND_LOG\"\n")
			// Fixtures hold "version=N"; "corrupt" fails the integrity check.
			write(filepath.Join(bin, "sqlite3"), `#!/bin/sh
[ "$1" = "-readonly" ] || exit 1
case "$3" in
    'PRAGMA user_version;') sed -n 's/^version=\([0-9]*\).*/\1/p' "$2" ;;
    'PRAGMA integrity_check;') if grep -q corrupt "$2"; then echo corrupt; else echo ok; fi ;;
    *) exit 1 ;;
esac
`)
			target := filepath.Join(dir, "bookmarkd")
			write(target, "new binary")
			if !tt.noPrevious {
				write(target+".previous", "old binary")
			}
			logPath := filepath.Join(dir, "commands.log")
			write(logPath, "")

			remote := strings.ReplaceAll(body, "/etc/bookmarks/bookmarkd.env", config)
			run := func(args ...string) (string, error) {
				cmd := exec.Command("sh", append([]string{"-eu", "-s", "--"}, args...)...)
				cmd.Stdin = strings.NewReader(remote)
				cmd.Env = append(os.Environ(),
					"PATH="+bin+":"+os.Getenv("PATH"),
					"TEST_COMMAND_LOG="+logPath,
				)
				output, err := cmd.CombinedOutput()
				return string(output), err
			}

			output, err := run("plan", "bookmarkd", target)
			if tt.wantPlan == "" {
				if err == nil {
					t.Fatalf("plan succeeded, want error\n%s", output)
				}
			} else if err != nil || !strings.Contains(output, "\nmode="+tt.wantPlan+"\n") {
				t.Fatalf("plan error = %v, want mode=%s\n%s", err, tt.wantPlan, output)
			}

			output, err = run("apply", "bookmarkd", target, tt.applyMode)
			if (err != nil) != tt.wantError {
				t.Fatalf("apply error = %v, want error %t\n%s", err, tt.wantError, output)
			}
			if got := read(logPath); got != tt.wantCommands {
				t.Fatalf("commands = %q, want %q\n%s", got, tt.wantCommands, output)
			}

			wantBinary := "new binary"
			if !tt.wantError {
				wantBinary = "old binary"
				if got := read(target + ".rolled-back"); got != "new binary" {
					t.Fatalf("rolled-back binary = %q", got)
				}
			}
			if got := read(target); got != wantBinary {
				t.Fatalf("installed binary = %q, want %q", got, wantBinary)
			}
			if got := read(db); got != tt.wantDB {
				t.Fatalf("database = %q, want %q", got, tt.wantDB)
			}

			if tt.applyMode == "restore" && !tt.wantError {
				saved, err := filepath.Glob(filepath.Join(backupDir, "rollback-*", "live database.db*"))
				if err != nil || len(saved) != 2 {
					t.Fatalf("safety copies = %v, error = %v, want database and WAL", saved, err)
				}
			}
		})
	}
}
