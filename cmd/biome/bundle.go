// Copyright 2021 Ross Light
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//		 https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"

	"go4.org/xdgdir"
	"zombiezen.com/go/biome"
	"zombiezen.com/go/biome/internal/gitglob"
	"zombiezen.com/go/log"
	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

const (
	ignoreFileName       = ".biomeignore"
	ignoreConfigFileName = "ignore"
)

// bundle writes a zip archive to out that contains any files that changed in
// src since the last call to bundle. prevStamps should be the previous return
// value of bundle, or an empty/nil map if this is the first call. toRemove is a
// list of files or directories that should be removed before extracting the
// resulting zip archive.
func bundle(ctx context.Context, out io.Writer, src fs.FS, globalIgnore []gitglob.Pattern, prevStamps map[string]string) (newStamps map[string]string, toRemove []string, err error) {
	ignorePatterns := append([]gitglob.Pattern(nil), globalIgnore...)
	ignorePatterns, err = readLocalIgnore(ignorePatterns, src)
	if err != nil {
		return nil, nil, err
	}

	newStamps = make(map[string]string)
	zw := zip.NewWriter(out)
	err = fs.WalkDir(src, ".", func(path string, ent fs.DirEntry, err error) error {
		if err != nil {
			log.Warnf(ctx, "Could not list %s: %v", path, err)
			return nil
		}
		if path == "." || path == ignoreFileName {
			return nil
		}
		if pat := gitglob.LastMatch(ignorePatterns, path, ent.Type()); pat != nil && !pat.IsNegated() {
			// Ignored.
			log.Debugf(ctx, "Ignored %s due to rule %q", path, pat)
			if ent.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		info, err := ent.Info()
		if err != nil {
			return err
		}

		oldStamp := prevStamps[path]
		if info.IsDir() {
			if oldStamp != dirStamp && oldStamp != "" {
				log.Debugf(ctx, "%s stamp %q -> %q", path, oldStamp, dirStamp)
				toRemove = append(toRemove, path)
			}
			newStamps[path] = dirStamp
			hdr, err := zip.FileInfoHeader(info)
			if err != nil {
				return err
			}
			hdr.Name = path + "/"
			_, err = zw.CreateHeader(hdr)
			return err
		}

		if info.Mode().Type() != 0 {
			return fmt.Errorf("%s: TODO(soon): only able to handle regular files", path)
		}
		if oldStamp == dirStamp {
			toRemove = append(toRemove, path)
		}
		newStamp := readStamp(src, path, info)
		newStamps[path] = newStamp
		if oldStamp == newStamp {
			log.Debugf(ctx, "%s has not changed", path)
			return nil
		}

		log.Debugf(ctx, "%s stamp %q -> %q", path, oldStamp, newStamp)
		f, err := src.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		hdr, err := zip.FileInfoHeader(info)
		if err != nil {
			return fmt.Errorf("%s: %v", path, err)
		}
		hdr.Name = path
		hdr.Method = zip.Deflate
		w, err := zw.CreateHeader(hdr)
		if err != nil {
			return fmt.Errorf("%s: %v", path, err)
		}
		if _, err := io.Copy(w, f); err != nil {
			return fmt.Errorf("%s: %v", path, err)
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, nil, err
	}
	for path := range prevStamps {
		if newStamps[path] == "" {
			toRemove = append(toRemove, path)
		}
	}
	return newStamps, toRemove, nil
}

func pushWorkDir(ctx context.Context, conn *sqlite.Conn, rec *biomeRecord, bio biome.Biome) (err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("push %s to %s: %v", rec.rootHostDir, rec.id, err)
		}
	}()
	ignorePatterns, err := readGlobalIgnore()
	if err != nil {
		return err
	}

	defer sqlitex.Save(conn)(&err)

	// Read previous stamps.
	const prevStampsQuery = `select "path", "stamp" from "local_files" where "biome_id" = ?;`
	prevStamps := make(map[string]string)
	err = sqlitex.ExecTransient(conn, prevStampsQuery, func(stmt *sqlite.Stmt) error {
		prevStamps[stmt.ColumnText(0)] = stmt.ColumnText(1)
		return nil
	}, rec.id)
	if err != nil {
		return err
	}

	// Copy bundle to HOME.
	zipName, err := genHexDigits(8)
	if err != nil {
		return err
	}
	zipName += ".zip"
	zipPath := biome.JoinPath(bio.Describe(), bio.Dirs().Home, zipName)
	pr, pw := io.Pipe()
	writeErrChan := make(chan error)
	go func() {
		err := biome.WriteFile(ctx, bio, zipPath, pr)
		pr.CloseWithError(err)
		writeErrChan <- err
	}()
	defer func() {
		err := bio.Run(ctx, &biome.Invocation{
			Argv:   []string{"rm", "-f", zipPath},
			Stdout: os.Stderr,
			Stderr: os.Stderr,
		})
		if err != nil {
			log.Warnf(ctx, "Failed to clean up %s in biome: %v", zipPath, err)
		}
	}()
	newStamps, toRemove, err := bundle(ctx, pw, os.DirFS(rec.rootHostDir), ignorePatterns, prevStamps)
	pw.Close()
	writeErr := <-writeErrChan
	if err != nil {
		return err
	}
	if writeErr != nil {
		return writeErr
	}

	// Remove any files first.
	if len(toRemove) > 0 {
		rmArgs := make([]string, 0, len(toRemove)+3)
		rmArgs = append(rmArgs, "rm", "-r", "-f")
		for _, path := range toRemove {
			rmArgs = append(rmArgs, biome.FromSlash(bio.Describe(), path))
		}
		err = bio.Run(ctx, &biome.Invocation{
			Argv:   rmArgs,
			Stdout: os.Stderr,
			Stderr: os.Stderr,
		})
		if err != nil {
			return err
		}
	}

	// Unzip files.
	err = bio.Run(ctx, &biome.Invocation{
		Argv:   []string{"unzip", "-o", "-q", zipPath},
		Stdout: os.Stderr,
		Stderr: os.Stderr,
	})
	if err != nil {
		return err
	}

	// Record new stamps.
	err = sqlitex.ExecTransient(conn, `delete from "local_files" where "biome_id" = ?;`, nil, rec.id)
	if err != nil {
		return err
	}
	insertStampStmt := conn.Prep(`insert into "local_files" ("biome_id", "path", "stamp") values (?, ?, ?);`)
	insertStampStmt.BindText(1, rec.id)
	for path, stamp := range newStamps {
		insertStampStmt.BindText(2, path)
		insertStampStmt.BindText(3, stamp)
		if _, err := insertStampStmt.Step(); err != nil {
			return err
		}
		if err := insertStampStmt.Reset(); err != nil {
			return err
		}
	}

	return nil
}

// readStamp computes a checksum of a file based on its metadata.
// The checksum of a nonexistent or otherwise inaccessible file is "0".
func readStamp(fsys fs.FS, path string, info fs.FileInfo) string {
	pre := marshalStamp(info)
	if info.Mode().Type() != fs.ModeSymlink {
		return pre
	}
	targetInfo, err := fs.Stat(fsys, path)
	if err != nil {
		return pre + "+0"
	}
	return pre + "+" + marshalStamp(targetInfo)
}

// dirStamp is the fake checksum value of a directory.
const dirStamp = "dir"

func marshalStamp(info fs.FileInfo) string {
	if info.IsDir() {
		return dirStamp
	}
	mtime := info.ModTime().UnixMicro()
	var ino, uid, gid uint64
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		ino = uint64(st.Ino)
		uid = uint64(st.Uid)
		gid = uint64(st.Gid)
	}
	return fmt.Sprintf("%d.%06d-%d-%d-%d-%d-%d",
		mtime/1e6, mtime%1e6,
		info.Size(),
		ino,
		info.Mode(),
		uid,
		gid,
	)
}

func readGlobalIgnore() ([]gitglob.Pattern, error) {
	paths := xdgdir.Config.SearchPaths()
	for i, dir := range paths {
		paths[i] = filepath.Join(dir, configSubdirName, ignoreConfigFileName)
	}
	return gitglob.ParseFiles(paths...)
}

func readLocalIgnore(dst []gitglob.Pattern, fsys fs.FS) ([]gitglob.Pattern, error) {
	data, err := fs.ReadFile(fsys, ignoreFileName)
	if errors.Is(err, fs.ErrNotExist) {
		return dst, nil
	}
	if err != nil {
		return dst, err
	}
	for _, line := range bytes.Split(data, []byte("\n")) {
		pat := gitglob.ParseLine(string(line))
		if pat.IsValid() {
			dst = append(dst, pat)
		}
	}
	return dst, nil
}
