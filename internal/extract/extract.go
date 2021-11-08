// Copyright 2021 Ross Light
// Copyright 2020 YourBase Inc.
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

package extract

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/yourbase/commons/xcontext"
	"zombiezen.com/go/biome"
	"zombiezen.com/go/biome/downloader"
	"zombiezen.com/go/log"
)

// Extract modes.
const (
	// Archive does not contain a top-level directory.
	Tarbomb = false
	// Remove the archive's top-level directory.
	StripTopDirectory = true
)

type Options struct {
	URL            string
	DestinationDir string

	Biome       biome.Biome
	Downloader  *downloader.Downloader
	Output      io.Writer
	ExtractMode bool
}

// Extract downloads the given URL and extracts it to the given directory in the biome.
func Extract(ctx context.Context, opts *Options) (err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("extract %s in %s: %w", opts.URL, opts.DestinationDir, err)
		}
	}()

	const (
		zipExt    = ".zip"
		tarXZExt  = ".tar.xz"
		tarGZExt  = ".tar.gz"
		tarBZ2Ext = ".tar.bz2"
	)
	const cleanupTimeout = 10 * time.Second
	exts := []string{
		zipExt,
		tarXZExt,
		tarGZExt,
		tarBZ2Ext,
	}
	var ext string
	for _, testExt := range exts {
		if strings.HasSuffix(opts.URL, testExt) {
			ext = testExt
			break
		}
	}
	if ext == "" {
		return fmt.Errorf("unknown extension")
	}

	f, err := opts.Downloader.Download(ctx, opts.URL)
	if err != nil {
		return err
	}
	defer f.Close()

	defer func() {
		// Attempt to clean up if unarchive fails.
		if err != nil {
			ctx, cancel := xcontext.KeepAlive(ctx, cleanupTimeout)
			defer cancel()
			rmErr := opts.Biome.Run(ctx, &biome.Invocation{
				Argv:   []string{"rm", "-rf", opts.DestinationDir},
				Stdout: opts.Output,
				Stderr: opts.Output,
			})
			if rmErr != nil {
				log.Warnf(ctx, "Failed to clean up %s: %v", opts.DestinationDir, rmErr)
			}
		}
	}()
	err = biome.MkdirAll(ctx, opts.Biome, opts.DestinationDir)
	if err != nil {
		return err
	}
	dstFile := opts.DestinationDir + ext
	defer func() {
		ctx, cancel := xcontext.KeepAlive(ctx, cleanupTimeout)
		defer cancel()
		rmErr := opts.Biome.Run(ctx, &biome.Invocation{
			Argv:   []string{"rm", "-f", dstFile},
			Stdout: opts.Output,
			Stderr: opts.Output,
		})
		if rmErr != nil {
			log.Warnf(ctx, "Failed to clean up %s: %v", dstFile, rmErr)
		}
	}()
	err = biome.WriteFile(ctx, opts.Biome, dstFile, f)
	if err != nil {
		return err
	}

	invoke := &biome.Invocation{
		Dir:    biome.AbsPath(opts.Biome, opts.DestinationDir),
		Stdout: opts.Output,
		Stderr: opts.Output,
	}
	absDstFile := biome.AbsPath(opts.Biome, dstFile)
	switch ext {
	case zipExt:
		invoke.Argv = []string{"unzip", "-q", absDstFile}
	case tarXZExt:
		invoke.Argv = []string{
			"tar",
			"-x", // extract
			"-J", // xz
			"-f", absDstFile,
		}
		if opts.ExtractMode == StripTopDirectory {
			invoke.Argv = append(invoke.Argv, "--strip-components", "1")
		}
	case tarGZExt:
		invoke.Argv = []string{
			"tar",
			"-x", // extract
			"-z", // gzip
			"-f", absDstFile,
		}
		if opts.ExtractMode == StripTopDirectory {
			invoke.Argv = append(invoke.Argv, "--strip-components", "1")
		}
	case tarBZ2Ext:
		invoke.Argv = []string{
			"tar",
			"-x", // extract
			"-j", // bzip2
			"-f", absDstFile,
		}
		if opts.ExtractMode == StripTopDirectory {
			invoke.Argv = append(invoke.Argv, "--strip-components", "1")
		}
	default:
		panic("unreachable")
	}
	if err := opts.Biome.Run(ctx, invoke); err != nil {
		return err
	}
	if ext == zipExt && opts.ExtractMode == StripTopDirectory {
		// There's no convenient way of stripping the top-level directory from an
		// unzip invocation, but we can move the files ourselves.
		size, err := f.Seek(0, io.SeekCurrent)
		if err != nil {
			return fmt.Errorf("determine archive size: %w", err)
		}
		zr, err := zip.NewReader(f, size)
		if err != nil {
			return err
		}
		root, names, err := topLevelZipFilenames(zr.File)
		if err != nil {
			return err
		}

		mvArgv := []string{"mv"}
		for _, name := range names {
			mvArgv = append(mvArgv, biome.JoinPath(opts.Biome.Describe(), root, name))
		}
		mvArgv = append(mvArgv, ".")
		err = opts.Biome.Run(ctx, &biome.Invocation{
			Argv:   mvArgv,
			Dir:    biome.AbsPath(opts.Biome, opts.DestinationDir),
			Stdout: opts.Output,
			Stderr: opts.Output,
		})
		if err != nil {
			return err
		}
		err = opts.Biome.Run(ctx, &biome.Invocation{
			Argv:   []string{"rmdir", root},
			Dir:    biome.AbsPath(opts.Biome, opts.DestinationDir),
			Stdout: opts.Output,
			Stderr: opts.Output,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// topLevelZipFilenames returns the names of the direct children of the root zip
// file directory.
func topLevelZipFilenames(files []*zip.File) (root string, names []string, _ error) {
	if len(files) == 0 {
		return "", nil, nil
	}
	i := strings.IndexByte(files[0].Name, '/')
	if i == -1 {
		return "", nil, fmt.Errorf("find zip root directory: %q not in a directory", files[0].Name)
	}
	root = files[0].Name[:i]
	prefix := files[0].Name[:i+1]
	for _, f := range files {
		if !strings.HasPrefix(f.Name, prefix) {
			return "", nil, fmt.Errorf("find zip root directory: %q not in directory %q", f.Name, root)
		}
		name := f.Name[i+1:]
		if nameEnd := strings.IndexByte(name, '/'); nameEnd != -1 {
			name = name[:nameEnd]
		}
		if name == root {
			return "", nil, fmt.Errorf("strip zip root directory: %q contains a file %q", name, name)
		}
		if name != "" && !stringInSlice(names, name) {
			names = append(names, name)
		}
	}
	return
}

func stringInSlice(slice []string, s string) bool {
	for _, elem := range slice {
		if elem == s {
			return true
		}
	}
	return false
}
