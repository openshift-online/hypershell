package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func checkoutGit(dir string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("prepare checkout: git %s: %w: %s", args[0], err, out)
	}
	return out, nil
}

// Each worker gets independent Git objects and working files. Never upload a
// linked worktree's host-only .git pointer or copy its credentials/hooks/config.
// The baseline index is HEAD; local staged and unstaged edits become unstaged.
func prepareCheckout(source, repository string) (string, func(), error) {
	parent, err := os.MkdirTemp("", "funnel-checkout-*")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() {
		if err := os.RemoveAll(parent); err != nil {
			fmt.Fprintln(os.Stderr, "checkout cleanup:", err)
		}
	}
	target := filepath.Join(parent, filepath.Base(source))
	if err := copyCheckout(source, target, "https://github.com/"+repository+".git"); err != nil {
		cleanup()
		return "", nil, err
	}
	return target, cleanup, nil
}

func copyCheckout(source, target, remote string) error {
	if _, err := checkoutGit(source, "clone", "--no-local", "--no-checkout", "--single-branch", "--", source, target); err != nil {
		return err
	}
	if _, err := checkoutGit(target, "remote", "set-url", "origin", remote); err != nil {
		return err
	}
	if _, err := checkoutGit(target, "read-tree", "HEAD"); err != nil {
		return err
	}
	files, err := checkoutGit(source, "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, name := range strings.Split(string(files), "\x00") {
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		if !filepath.IsLocal(name) {
			return fmt.Errorf("unsafe checkout path %q", name)
		}
		src, dst := filepath.Join(source, name), filepath.Join(target, name)
		info, err := os.Lstat(src)
		if os.IsNotExist(err) {
			continue
		} // Preserve local deletions.
		if err != nil {
			return err
		}
		if info.IsDir() {
			// Gitlinks need independent metadata too; never carry host submodule pointers.
			if _, err := os.Lstat(filepath.Join(src, ".git")); os.IsNotExist(err) {
				continue
			}
			url, err := checkoutGit(source, "config", "-f", ".gitmodules", "--get", "submodule."+name+".url")
			if err != nil {
				return fmt.Errorf("submodule %s: %w", name, err)
			}
			if err := copyCheckout(src, dst, strings.TrimSpace(string(url))); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			link, err := os.Readlink(src)
			if err != nil {
				return err
			}
			if err := os.Symlink(link, dst); err != nil {
				return err
			}
			continue
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("unsupported checkout file %s", name)
		}
		if err := copyCheckoutFile(src, dst, info.Mode().Perm()); err != nil {
			return err
		}
	}
	return nil
}

func copyCheckoutFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	closeErr := out.Close()
	if err != nil {
		return err
	}
	return closeErr
}
