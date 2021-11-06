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
	"context"
	"fmt"
	"io"
	"os"
	"runtime"
	"syscall"

	"github.com/spf13/cobra"
	"zombiezen.com/go/log"
	"zombiezen.com/go/sqlite/sqlitex"
)

type destroyCommand struct {
	biomeID string
}

func newDestroyCommand() *cobra.Command {
	c := new(destroyCommand)
	cmd := &cobra.Command{
		Use:                   "destroy [options] [--biome=ID]",
		DisableFlagsInUseLine: true,
		Short:                 "destroy a biome",
		Args:                  cobra.NoArgs,
		SilenceErrors:         true,
		SilenceUsage:          true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				c.biomeID = args[0]
			}
			return c.run(cmd.Context())
		},
	}
	cmd.Flags().StringVarP(&c.biomeID, "biome", "b", "", "biome to run inside")
	return cmd
}

func (c *destroyCommand) run(ctx context.Context) (err error) {
	db, err := openDB(ctx)
	if err != nil {
		return err
	}
	defer db.Close()

	defer sqlitex.Save(db)(&err)
	id, _, err := findBiome(db, c.biomeID)
	if err != nil {
		return fmt.Errorf("destroy %q: %v", id, err)
	}
	err = sqlitex.Exec(db, `delete from "biomes" where "id" = ?;`, nil, id)
	if err != nil {
		return fmt.Errorf("destroy %q: %v", id, err)
	}

	if dir, err := computeBiomeRoot(id); err != nil {
		log.Warnf(ctx, "Cleaning up biome: %v", err)
	} else if err := removeAll(ctx, dir); err != nil {
		return err
	}
	return nil
}

// removeAll removes path and any children it contains. It operates similar to
// os.RemoveAll, but also removes any write-protected files if possible.
//
// Copied from https://cs.opensource.google/go/go/+/refs/tags/go1.17.3:src/os/removeall_noat.go
func removeAll(ctx context.Context, path string) error {
	if path == "" {
		return &os.PathError{
			Op:   "remove",
			Path: path,
			Err:  fmt.Errorf("empty path"),
		}
	}

	// Simple case: if Remove works, we're done.
	err := os.Remove(path)
	if err == nil || os.IsNotExist(err) {
		return nil
	}

	// Otherwise, is this a directory we need to recurse into?
	dir, serr := os.Lstat(path)
	if serr != nil {
		if serr, ok := serr.(*os.PathError); ok && (os.IsNotExist(serr.Err) || serr.Err == syscall.ENOTDIR) {
			return nil
		}
		return serr
	}
	if !dir.IsDir() {
		// Not a directory; return the error from Remove.
		return err
	}
	if oldMode := dir.Mode(); oldMode.Perm()&0o222 == 0 {
		// No writable bits set on directory.
		// Attempt to set writable before recursing.
		newMode := oldMode | 0o200
		if chmodErr := os.Chmod(path, newMode); err != nil {
			log.Debugf(ctx, "chmod %v %s: %v", newMode, path, chmodErr)
		}
	}

	// Remove contents & return first error.
	err = nil
	for {
		fd, err := os.Open(path)
		if err != nil {
			if os.IsNotExist(err) {
				// Already deleted by someone else.
				return nil
			}
			return err
		}

		const reqSize = 1024
		var names []string
		var readErr error

		for {
			select {
			case <-ctx.Done():
				return &os.PathError{
					Op:   "remove",
					Path: path,
					Err:  ctx.Err(),
				}
			default:
			}
			numErr := 0
			names, readErr = fd.Readdirnames(reqSize)

			for _, name := range names {
				err1 := removeAll(ctx, path+string(os.PathSeparator)+name)
				if err == nil {
					err = err1
				}
				if err1 != nil {
					numErr++
				}
			}

			// If we can delete any entry, break to start new iteration.
			// Otherwise, we discard current names, get next entries and try deleting them.
			if numErr != reqSize {
				break
			}
		}

		// Removing files from the directory may have caused
		// the OS to reshuffle it. Simply calling Readdirnames
		// again may skip some entries. The only reliable way
		// to avoid this is to close and re-open the
		// directory. See golang.org/issue/20841.
		fd.Close()

		if readErr == io.EOF {
			break
		}
		// If Readdirnames returned an error, use it.
		if err == nil {
			err = readErr
		}
		if len(names) == 0 {
			break
		}

		// We don't want to re-open unnecessarily, so if we
		// got fewer than request names from Readdirnames, try
		// simply removing the directory now. If that
		// succeeds, we are done.
		if len(names) < reqSize {
			err1 := os.Remove(path)
			if err1 == nil || os.IsNotExist(err1) {
				return nil
			}

			if err != nil {
				// We got some error removing the
				// directory contents, and since we
				// read fewer names than we requested
				// there probably aren't more files to
				// remove. Don't loop around to read
				// the directory again. We'll probably
				// just get the same error.
				return err
			}
		}
	}

	// Remove directory.
	err1 := os.Remove(path)
	if err1 == nil || os.IsNotExist(err1) {
		return nil
	}
	if runtime.GOOS == "windows" && os.IsPermission(err1) {
		if fs, err := os.Stat(path); err == nil {
			if err = os.Chmod(path, 0o200|fs.Mode()); err == nil {
				err1 = os.Remove(path)
			}
		}
	}
	if err == nil {
		err = err1
	}
	return err
}

// endsWithDot reports whether the final component of path is ".".
func endsWithDot(path string) bool {
	if path == "." {
		return true
	}
	if len(path) >= 2 && path[len(path)-1] == '.' && os.IsPathSeparator(path[len(path)-2]) {
		return true
	}
	return false
}
