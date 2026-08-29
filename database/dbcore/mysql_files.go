package dbcore

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/komari-monitor/komari/cmd/flags"
	"github.com/komari-monitor/komari/database/models"
	logger "github.com/komari-monitor/komari/utils/log"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const mysqlPersistentFileChunkSize = 1 << 20

var mysqlPersistentFileRoots = []string{
	"favicon.ico",
	"font.ttf",
	"GeoLite2-Country.mmdb",
	"theme",
	"plugin",
	"plugin-data",
}

var mysqlPersistentFileTombstoneRoots = []string{
	"theme",
	"plugin",
	"plugin-data",
}

// RestorePersistentFiles restores the MySQL-backed filesystem mirror into
// dataDir. Existing local files win, so a mounted data volume is never
// overwritten; missing assets are recreated after a stateless redeploy.
func RestorePersistentFiles(ctx context.Context, dataDir string) error {
	if !flags.IsMySQL() {
		return nil
	}
	db := GetDBInstance().WithContext(ctx)
	if !db.Migrator().HasTable(&models.PersistentFile{}) {
		return nil
	}
	var files []models.PersistentFile
	if err := db.Order("path").Find(&files).Error; err != nil {
		return fmt.Errorf("list persistent files: %w", err)
	}
	for _, file := range files {
		target, err := persistentFileTarget(dataDir, file.Path)
		if err != nil {
			return err
		}
		if _, err := os.Stat(target); err == nil {
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("stat persistent file %s: %w", file.Path, err)
		}
		if err := restorePersistentFile(db, target, file); err != nil {
			return err
		}
	}
	return nil
}

// SyncPersistentFiles snapshots every configured file root to MySQL. It uses
// a manifest comparison and only rewrites changed files; stale database rows
// are removed after the full local walk succeeds.
func SyncPersistentFiles(ctx context.Context, dataDir string) error {
	if !flags.IsMySQL() {
		return nil
	}
	db := GetDBInstance().WithContext(ctx)
	local, err := collectPersistentFiles(dataDir)
	if err != nil {
		return err
	}
	var existing []models.PersistentFile
	if err := db.Find(&existing).Error; err != nil {
		return fmt.Errorf("load persistent file manifest: %w", err)
	}
	existingByPath := make(map[string]models.PersistentFile, len(existing))
	for _, file := range existing {
		existingByPath[file.Path] = file
	}
	for _, file := range local {
		old, found := existingByPath[file.meta.Path]
		if found && old.SHA256 == file.meta.SHA256 && old.Size == file.meta.Size && old.Mode == file.meta.Mode {
			delete(existingByPath, file.meta.Path)
			continue
		}
		if err := storePersistentFile(db, file); err != nil {
			return err
		}
		delete(existingByPath, file.meta.Path)
	}
	stale := make([]string, 0, len(existingByPath))
	for path := range existingByPath {
		target, targetErr := persistentFileTarget(dataDir, path)
		if targetErr != nil {
			return targetErr
		}
		if persistentFileRootPresent(dataDir, path) {
			if _, statErr := os.Stat(target); errors.Is(statErr, os.ErrNotExist) {
				stale = append(stale, path)
			} else if statErr != nil {
				return fmt.Errorf("stat stale persistent file %s: %w", path, statErr)
			}
		}
	}
	if len(stale) > 0 {
		if err := db.Where("path IN ?", stale).Delete(&models.PersistentFile{}).Error; err != nil {
			return fmt.Errorf("delete stale persistent files: %w", err)
		}
	}
	return nil
}

func persistentFileRootPresent(dataDir, relative string) bool {
	first := strings.SplitN(filepath.ToSlash(relative), "/", 2)[0]
	for _, root := range mysqlPersistentFileTombstoneRoots {
		if strings.TrimSuffix(root, "/") != first {
			continue
		}
		_, err := os.Stat(filepath.Join(dataDir, filepath.FromSlash(root)))
		return err == nil
	}
	for _, root := range mysqlPersistentFileRoots {
		if strings.TrimSuffix(root, "/") == first {
			// Singleton assets (favicon/font/GeoIP) live directly under dataDir.
			// Restore runs before synchronization, so a missing file here is an
			// intentional runtime deletion and should remove its MySQL mirror.
			_, err := os.Stat(dataDir)
			return err == nil
		}
	}
	return false
}

type localPersistentFile struct {
	meta models.PersistentFile
	path string
}

func collectPersistentFiles(dataDir string) ([]localPersistentFile, error) {
	var files []localPersistentFile
	for _, root := range mysqlPersistentFileRoots {
		path := filepath.Join(dataDir, filepath.FromSlash(root))
		info, err := os.Stat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("stat persistent root %s: %w", root, err)
		}
		if info.IsDir() {
			err = filepath.Walk(path, func(current string, info os.FileInfo, walkErr error) error {
				if walkErr != nil {
					return walkErr
				}
				if info.IsDir() || !info.Mode().IsRegular() {
					return nil
				}
				file, err := inspectPersistentFile(dataDir, current, info)
				if err == nil {
					files = append(files, file)
				}
				return err
			})
			if err != nil {
				return nil, fmt.Errorf("walk persistent root %s: %w", root, err)
			}
		} else if info.Mode().IsRegular() {
			file, err := inspectPersistentFile(dataDir, path, info)
			if err != nil {
				return nil, err
			}
			files = append(files, file)
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].meta.Path < files[j].meta.Path })
	return files, nil
}

func inspectPersistentFile(dataDir, path string, info os.FileInfo) (localPersistentFile, error) {
	relative, err := filepath.Rel(dataDir, path)
	if err != nil {
		return localPersistentFile{}, err
	}
	relative = filepath.ToSlash(relative)
	if _, err := persistentFileTarget(dataDir, relative); err != nil {
		return localPersistentFile{}, err
	}
	input, err := os.Open(path)
	if err != nil {
		return localPersistentFile{}, fmt.Errorf("open persistent file %s: %w", relative, err)
	}
	hash := sha256.New()
	_, copyErr := io.Copy(hash, input)
	closeErr := input.Close()
	if copyErr != nil {
		return localPersistentFile{}, fmt.Errorf("hash persistent file %s: %w", relative, copyErr)
	}
	if closeErr != nil {
		return localPersistentFile{}, closeErr
	}
	return localPersistentFile{path: path, meta: models.PersistentFile{
		Path: relative, Mode: uint32(info.Mode().Perm()), Size: info.Size(),
		SHA256: hex.EncodeToString(hash.Sum(nil)), ChunkSize: mysqlPersistentFileChunkSize,
		UpdatedAt: info.ModTime().UTC(),
	}}, nil
}

func storePersistentFile(db *gorm.DB, file localPersistentFile) error {
	input, err := os.Open(file.path)
	if err != nil {
		return fmt.Errorf("open persistent file %s: %w", file.meta.Path, err)
	}
	defer input.Close()
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "path"}},
			DoUpdates: clause.AssignmentColumns([]string{"mode", "size", "sha256", "chunk_size", "updated_at"}),
		}).Create(&file.meta).Error; err != nil {
			return fmt.Errorf("store manifest for %s: %w", file.meta.Path, err)
		}
		if err := tx.Where("path = ?", file.meta.Path).Delete(&models.PersistentFileChunk{}).Error; err != nil {
			return fmt.Errorf("clear chunks for %s: %w", file.meta.Path, err)
		}
		reader := bufio.NewReaderSize(input, mysqlPersistentFileChunkSize)
		hash := sha256.New()
		var total int64
		for index := uint32(0); ; index++ {
			data := make([]byte, mysqlPersistentFileChunkSize)
			count, readErr := io.ReadFull(reader, data)
			if readErr != nil && readErr != io.EOF && readErr != io.ErrUnexpectedEOF {
				return fmt.Errorf("read persistent file %s: %w", file.meta.Path, readErr)
			}
			if count > 0 {
				_, _ = hash.Write(data[:count])
				total += int64(count)
				chunk := models.PersistentFileChunk{Path: file.meta.Path, Index: index, Data: data[:count]}
				if err := tx.Create(&chunk).Error; err != nil {
					return fmt.Errorf("store chunk %d for %s: %w", index, file.meta.Path, err)
				}
			}
			if readErr == io.EOF || readErr == io.ErrUnexpectedEOF {
				break
			}
		}
		if total != file.meta.Size || hex.EncodeToString(hash.Sum(nil)) != file.meta.SHA256 {
			return fmt.Errorf("persistent file %s changed while it was being synchronized", file.meta.Path)
		}
		return nil
	})
}

func restorePersistentFile(db *gorm.DB, target string, file models.PersistentFile) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(target), ".komari-restore-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	hash := sha256.New()
	writer := io.MultiWriter(temporary, hash)
	rows, err := db.Model(&models.PersistentFileChunk{}).Where("path = ?", file.Path).Order("chunk_index").Rows()
	if err != nil {
		_ = temporary.Close()
		return fmt.Errorf("load chunks for %s: %w", file.Path, err)
	}
	defer rows.Close()
	var written int64
	for rows.Next() {
		var chunk models.PersistentFileChunk
		if err := db.ScanRows(rows, &chunk); err != nil {
			_ = temporary.Close()
			return fmt.Errorf("scan chunk for %s: %w", file.Path, err)
		}
		count, err := writer.Write(chunk.Data)
		written += int64(count)
		if err != nil {
			_ = temporary.Close()
			return fmt.Errorf("restore persistent file %s: %w", file.Path, err)
		}
	}
	if err := rows.Err(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("iterate chunks for %s: %w", file.Path, err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if written != file.Size || hex.EncodeToString(hash.Sum(nil)) != file.SHA256 {
		return fmt.Errorf("persistent file %s failed size or checksum validation", file.Path)
	}
	mode := os.FileMode(file.Mode)
	if mode == 0 {
		mode = 0o644
	}
	if err := os.Chmod(temporaryPath, mode); err != nil {
		return err
	}
	if !file.UpdatedAt.IsZero() {
		_ = os.Chtimes(temporaryPath, file.UpdatedAt, file.UpdatedAt)
	}
	if err := os.Rename(temporaryPath, target); err != nil {
		return fmt.Errorf("publish persistent file %s: %w", file.Path, err)
	}
	logger.Infof("dbcore", "Restored MySQL-backed file %s", file.Path)
	return nil
}

func persistentFileTarget(dataDir, relative string) (string, error) {
	relative = filepath.Clean(filepath.FromSlash(strings.TrimSpace(relative)))
	if relative == "." || filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid persistent file path %q", relative)
	}
	base, err := filepath.Abs(dataDir)
	if err != nil {
		return "", err
	}
	target := filepath.Join(base, relative)
	rel, err := filepath.Rel(base, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("persistent file path escapes data directory: %q", relative)
	}
	return target, nil
}

// SyncPersistentFilesWithTimeout is a convenience for shutdown paths that may
// receive a nil or already-expired context.
func SyncPersistentFilesWithTimeout(ctx context.Context, dataDir string) error {
	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
	}
	return SyncPersistentFiles(ctx, dataDir)
}
