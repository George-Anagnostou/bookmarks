package scripts

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Exercise the deployment's remote shell body locally with temporary files and
// fake system commands. These tests never connect to a host or control services.
func TestDeployRemote(t *testing.T) {
	source, err := os.ReadFile("deploy-bookmarkd.sh")
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
		database      bool
		version       string
		missingConfig bool
		failCommand   string
		noBackup      bool
		wantError     bool
		wantInstalled bool
		wantCommands  string
	}{
		{
			name: "back up configured database before replacing binary", database: true, version: "1",
			wantInstalled: true,
			wantCommands:  "stop bookmarkd\nrestart bookmarks-backup.service\nstart bookmarkd\n--no-pager --full status bookmarkd\n",
		},
		{
			name: "first deploy without database", wantInstalled: true,
			wantCommands: "stop bookmarkd\nstart bookmarkd\n--no-pager --full status bookmarkd\n",
		},
		{
			name: "backup failure preserves old binary", database: true, version: "1",
			failCommand: "restart bookmarks-backup.service", wantError: true,
			wantCommands: "stop bookmarkd\nrestart bookmarks-backup.service\n",
		},
		{
			name: "missing new backup preserves old binary", database: true, version: "1",
			noBackup: true, wantError: true,
			wantCommands: "stop bookmarkd\nrestart bookmarks-backup.service\n",
		},
		{
			name: "unversioned database rejected before stopping", database: true, version: "0",
			wantError: true,
		},
		{
			name: "unreadable configuration fails before stopping", missingConfig: true,
			wantError: true,
		},
		{
			name: "startup failure retains previous binary", database: true, version: "1",
			failCommand: "start bookmarkd", wantError: true, wantInstalled: true,
			wantCommands: "stop bookmarkd\nrestart bookmarks-backup.service\nstart bookmarkd\n",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			bin := filepath.Join(dir, "bin")
			if err := os.Mkdir(bin, 0o755); err != nil {
				t.Fatal(err)
			}
			write := func(path, contents string) {
				t.Helper()
				if err := os.WriteFile(path, []byte(contents), 0o700); err != nil {
					t.Fatal(err)
				}
			}
			db := filepath.Join(dir, "configured database.db")
			if tt.database {
				write(db, "existing database")
			}
			backupDir := filepath.Join(dir, "backups")
			if err := os.Mkdir(backupDir, 0o755); err != nil {
				t.Fatal(err)
			}
			config := filepath.Join(dir, "bookmarkd.env")
			if !tt.missingConfig {
				write(config, "BOOKMARKS_DBPATH='"+db+"'\nBOOKMARKS_BACKUP_DIR='"+backupDir+"'\n")
			}
			backup := filepath.Join(bin, "bookmarks-backup")
			write(backup, "#!/bin/sh\nexit 99\n") // Must use the service, not this executable directly.
			write(filepath.Join(bin, "id"), "#!/bin/sh\necho 0\n")
			write(filepath.Join(bin, "install"), "#!/bin/sh\nexec /bin/cp \"$7\" \"$8\"\n")
			write(filepath.Join(bin, "systemctl"), `#!/bin/sh
echo "$*" >> "$TEST_COMMAND_LOG"
if [ "$*" = "$TEST_FAIL_COMMAND" ]; then exit 1; fi
if [ "$*" = "restart bookmarks-backup.service" ] && [ "$TEST_NO_BACKUP" = "" ]; then
    echo "version=$TEST_SCHEMA_VERSION" > "$TEST_BACKUP_DIR/bookmarks-20261022T000000Z.db"
fi
`)
			// Backup fixtures hold "version=N" so the fake can report their version.
			write(filepath.Join(bin, "sqlite3"), `#!/bin/sh
[ "$1" = "-readonly" ] || exit 1
if [ "$2" != "$TEST_DB_PATH" ]; then
    [ "$3" = 'PRAGMA user_version;' ] && sed -n 's/^version=//p' "$2"
    exit
fi
case "$3" in
    'PRAGMA user_version;') echo "$TEST_SCHEMA_VERSION" ;;
    'SELECT count(*) FROM sqlite_schema'*) echo 2 ;;
    *) exit 1 ;;
esac
`)
			// An older backup must not be mistaken for the pre-deploy backup.
			write(filepath.Join(backupDir, "bookmarks-20261001T000000Z.db"), "version=1\n")
			target := filepath.Join(dir, "bookmarkd")
			candidate := filepath.Join(dir, "candidate")
			write(target, "old binary")
			write(candidate, "new binary")
			logPath := filepath.Join(dir, "commands.log")
			write(logPath, "")

			remote := strings.ReplaceAll(body, "/etc/bookmarks/bookmarkd.env", config)
			remote = strings.ReplaceAll(remote, "/usr/local/sbin/bookmarks-backup", backup)
			noBackup := ""
			if tt.noBackup {
				noBackup = "1"
			}
			cmd := exec.Command("sh", "-eu", "-s", "--", candidate, "bookmarkd", target)
			cmd.Stdin = strings.NewReader(remote)
			cmd.Env = append(os.Environ(),
				"PATH="+bin+":"+os.Getenv("PATH"),
				"TEST_DB_PATH="+db,
				"TEST_SCHEMA_VERSION="+tt.version,
				"TEST_COMMAND_LOG="+logPath,
				"TEST_FAIL_COMMAND="+tt.failCommand,
				"TEST_BACKUP_DIR="+backupDir,
				"TEST_NO_BACKUP="+noBackup,
			)
			output, err := cmd.CombinedOutput()
			if (err != nil) != tt.wantError {
				t.Fatalf("deployment error = %v, want error %t\n%s", err, tt.wantError, output)
			}
			commands, err := os.ReadFile(logPath)
			if err != nil {
				t.Fatal(err)
			}
			if string(commands) != tt.wantCommands {
				t.Fatalf("commands = %q, want %q\n%s", commands, tt.wantCommands, output)
			}
			installed, err := os.ReadFile(target)
			if err != nil {
				t.Fatal(err)
			}
			wantBinary := "old binary"
			if tt.wantInstalled {
				wantBinary = "new binary"
				previous, err := os.ReadFile(target + ".previous")
				if err != nil || string(previous) != "old binary" {
					t.Fatalf("previous binary = %q, error = %v", previous, err)
				}
			}
			if string(installed) != wantBinary {
				t.Fatalf("installed binary = %q, want %q", installed, wantBinary)
			}

			record, err := os.ReadFile(filepath.Join(backupDir, "pre-deploy.env"))
			if tt.wantInstalled && tt.database {
				want := "PRE_DEPLOY_SCHEMA_VERSION=" + tt.version + "\nPRE_DEPLOY_BACKUP=" +
					filepath.Join(backupDir, "bookmarks-20261022T000000Z.db") + "\n"
				if err != nil || string(record) != want {
					t.Fatalf("pre-deploy record = %q, error = %v, want %q", record, err, want)
				}
				saved, err := os.ReadFile(filepath.Join(backupDir, "pre-deploy.db"))
				if err != nil || string(saved) != "version="+tt.version+"\n" {
					t.Fatalf("pre-deploy backup = %q, error = %v", saved, err)
				}
			} else if err == nil {
				t.Fatalf("unexpected pre-deploy record %q", record)
			}
		})
	}
}
