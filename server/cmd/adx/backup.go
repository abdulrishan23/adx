package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/caarlos0/env/v11"

	"github.com/WokeWorks/adx/server/store"
)

// runBackup copies the database and the assets directory into a new,
// timestamped directory under dest. It reads the same ADX_DB_PATH and
// ADX_ASSETS_DIR the server uses, so it can run from a cron job next to the
// server. The encryption key is never copied: back it up separately
// (docs/runbooks/backup-restore.md).
func runBackup(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: adx backup DEST_DIR")
	}
	cfg, err := env.ParseAs[config]()
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	dir, err := backup(context.Background(), cfg.DBPath, cfg.AssetsDir, args[0], now)
	if err != nil {
		return err
	}
	fmt.Println(dir)
	pruned, err := prune(args[0], now, cfg.BackupRetentionDays)
	if err != nil {
		return fmt.Errorf("backup written, pruning failed: %w", err)
	}
	for _, p := range pruned {
		fmt.Println("pruned", p)
	}
	return nil
}

// prune removes backup directories older than days, by the UTC stamp in
// their name. Zero days keeps everything; a directory whose name is not a
// stamp is never touched. Pruning runs only after a new backup succeeded, so
// retention never leaves fewer backups than it found.
func prune(dest string, now time.Time, days int) ([]string, error) {
	if days <= 0 {
		return nil, nil
	}
	entries, err := os.ReadDir(dest)
	if err != nil {
		return nil, err
	}
	cutoff := now.Add(-time.Duration(days) * 24 * time.Hour)
	var pruned []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		stamp, err := time.Parse("20060102T150405Z", e.Name())
		if err != nil || !stamp.Before(cutoff) {
			continue
		}
		if err := os.RemoveAll(filepath.Join(dest, e.Name())); err != nil {
			return pruned, err
		}
		pruned = append(pruned, e.Name())
	}
	return pruned, nil
}

func backup(ctx context.Context, dbPath, assetsDir, dest string, now time.Time) (string, error) {
	dir := filepath.Join(dest, now.Format("20060102T150405Z"))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	db, err := store.Open(dbPath)
	if err != nil {
		return "", err
	}
	defer db.Close()
	if err := db.BackupTo(ctx, filepath.Join(dir, "adx.db")); err != nil {
		return "", err
	}
	if err := copyTree(assetsDir, filepath.Join(dir, "assets")); err != nil {
		return "", fmt.Errorf("copy assets: %w", err)
	}
	return dir, nil
}

// copyTree copies regular files under src to dst. A missing src is fine: a
// server that has never stored an asset has no directory yet.
func copyTree(src, dst string) error {
	if _, err := os.Stat(src); errors.Is(err, fs.ErrNotExist) {
		return os.MkdirAll(dst, 0o700)
	}
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, in); err != nil {
			out.Close()
			return err
		}
		return out.Close()
	})
}
