package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckoutFromDirtyLinkedWorktree(t *testing.T) {
	main := t.TempDir()
	git := func(dir string, args ...string) string {
		t.Helper()
		b, e := checkoutGit(dir, args...)
		if e != nil {
			t.Fatal(e)
		}
		return string(b)
	}
	write := func(dir, name, body string) {
		t.Helper()
		if e := os.WriteFile(filepath.Join(dir, name), []byte(body), 0755); e != nil {
			t.Fatal(e)
		}
	}
	git(main, "init", "-b", "main")
	git(main, "config", "user.name", "Test")
	git(main, "config", "user.email", "test@example.com")
	write(main, "tracked", "original")
	write(main, "deleted", "remove")
	write(main, ".gitignore", "secret.env\n")
	git(main, "add", ".")
	git(main, "commit", "-m", "initial")
	source := filepath.Join(t.TempDir(), "linked")
	git(main, "worktree", "add", "-b", "local", source)
	write(source, "tracked", "local edit")
	write(source, "added", "staged")
	git(source, "add", "added")
	write(source, "untracked", "local new file")
	write(source, "secret.env", "not uploaded")
	if e := os.Remove(filepath.Join(source, "deleted")); e != nil {
		t.Fatal(e)
	}
	if e := os.Symlink("tracked", filepath.Join(source, "link")); e != nil {
		t.Fatal(e)
	}
	git(source, "config", "credential.helper", "host-only-secret")
	before := git(source, "status", "--porcelain")
	target, cleanup, err := prepareCheckout(source, "test/repo")
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	for name, want := range map[string]string{"tracked": "local edit", "added": "staged", "untracked": "local new file"} {
		b, e := os.ReadFile(filepath.Join(target, name))
		if e != nil || string(b) != want {
			t.Fatalf("%s: %s %v", name, b, e)
		}
	}
	for _, name := range []string{"deleted", "secret.env", ".git/objects/info/alternates"} {
		if _, e := os.Stat(filepath.Join(target, name)); !os.IsNotExist(e) {
			t.Fatalf("unexpected file %s: %v", name, e)
		}
	}
	if info, e := os.Stat(filepath.Join(target, ".git")); e != nil || !info.IsDir() {
		t.Fatalf("not independent git metadata: %v", e)
	}
	if got := strings.TrimSpace(git(target, "remote", "get-url", "origin")); got != "https://github.com/test/repo.git" {
		t.Fatal(got)
	}
	if _, e := checkoutGit(target, "config", "--local", "--get", "credential.helper"); e == nil {
		t.Fatal("copied host credentials")
	}
	if got := git(source, "status", "--porcelain"); got != before {
		t.Fatalf("source changed: %s", got)
	}
	// Prove metadata and contents survive moving away from host worktree paths.
	moved := filepath.Join(t.TempDir(), "uploaded")
	if e := os.Rename(target, moved); e != nil {
		t.Fatal(e)
	}
	git(moved, "fsck", "--full")
	git(moved, "diff", "--stat")
	if link, e := os.Readlink(filepath.Join(moved, "link")); e != nil || link != "tracked" {
		t.Fatalf("symlink: %s %v", link, e)
	}
	write(moved, "tracked", "worker edit")
	if b, _ := os.ReadFile(filepath.Join(source, "tracked")); string(b) != "local edit" {
		t.Fatal("worker modified source")
	}
}
