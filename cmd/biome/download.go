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
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"zombiezen.com/go/biome"
	"zombiezen.com/go/log"
	"zombiezen.com/go/sqlite/sqlitex"
)

type downloadCommand struct {
	biomeID string
	files   []string
}

func newDownloadCommand() *cobra.Command {
	c := new(downloadCommand)
	cmd := &cobra.Command{
		Use:                   "download [options] FILE [...]",
		DisableFlagsInUseLine: true,
		Short:                 "copy a file from the biome into the working directory",
		Args:                  cobra.MinimumNArgs(1),
		SilenceErrors:         true,
		SilenceUsage:          true,
		RunE: func(cmd *cobra.Command, args []string) error {
			c.files = args
			return c.run(cmd.Context())
		},
	}
	cmd.Flags().StringVarP(&c.biomeID, "biome", "b", "", "biome to run inside")
	return cmd
}

func (c *downloadCommand) run(ctx context.Context) error {
	var rec *biomeRecord
	var bio biome.Biome
	err := func() (err error) {
		db, err := openDB(ctx)
		if err != nil {
			return err
		}
		defer db.Close()
		endFn, err := sqlitex.ImmediateTransaction(db)
		if err != nil {
			return err
		}
		defer endFn(&err)
		rec, err = findBiome(db, c.biomeID)
		if err != nil {
			return err
		}
		bio, err = rec.setup(ctx, db)
		if err != nil {
			return err
		}
		return nil
	}()
	if err != nil {
		return err
	}

	// Create zip file of requested files and directories.
	zipName, err := genHexDigits(8)
	if err != nil {
		return err
	}
	zipName += ".zip"
	zipPath := biome.JoinPath(bio.Describe(), bio.Dirs().Home, zipName)
	zipArgs := make([]string, 0, len(c.files)+4)
	zipArgs = append(zipArgs, "zip", "-q", "-r", zipPath)
	for _, file := range c.files {
		absFile, err := filepath.Abs(file)
		if err != nil {
			return err
		}
		relFile, err := filepath.Rel(rec.rootHostDir, absFile)
		if err != nil {
			return err
		}
		if !isSubFilepath(relFile) {
			return fmt.Errorf("%s: not inside %s", file, rec.rootHostDir)
		}
		biomePath := biome.FromSlash(bio.Describe(), filepath.ToSlash(relFile))
		zipArgs = append(zipArgs, biomePath)
	}
	err = bio.Run(ctx, &biome.Invocation{
		Argv:   zipArgs,
		Stdout: os.Stderr,
		Stderr: os.Stderr,
	})
	defer func() {
		log.Debugf(ctx, "Cleaning up %s inside biome", zipPath)
		output := new(strings.Builder)
		err := bio.Run(ctx, &biome.Invocation{
			Argv:   []string{"rm", "-f", "--", zipPath},
			Stdout: output,
			Stderr: output,
		})
		if err != nil {
			if output.Len() == 0 {
				log.Warnf(ctx, "Clean up archive %s in biome: %v", zipPath, err)
			} else {
				log.Warnf(ctx, "Clean up archive %s in biome: %v\n%s", zipPath, err, output)
			}
		}
	}()
	if err != nil {
		return err
	}

	// Download zip file.
	tempZip, err := os.CreateTemp("", "zombiezen-biome-*.zip")
	if err != nil {
		return err
	}
	hostZipPath := tempZip.Name()
	log.Debugf(ctx, "Downloading to %s on host", hostZipPath)
	defer func() {
		log.Debugf(ctx, "Cleaning up %s on host", hostZipPath)
		if err := tempZip.Close(); err != nil {
			log.Debugf(ctx, "Closing biome download archive: %v", err)
		}
		if err := os.Remove(hostZipPath); err != nil {
			log.Warnf(ctx, "Clean up biome download archive: %v", err)
		}
	}()
	rc, err := biome.OpenFile(ctx, bio, zipPath)
	if err != nil {
		return err
	}
	_, err = io.Copy(tempZip, rc)
	closeErr := rc.Close()
	if closeErr != nil {
		log.Debugf(ctx, "Closing biome-created archive: %v", closeErr)
	}
	if err != nil {
		return fmt.Errorf("download %s from biome: %w", zipPath, err)
	}

	// Extract zip file.
	log.Debugf(ctx, "Extracting to %s on host", rec.rootHostDir)
	unzipCmd := exec.CommandContext(ctx, "unzip", "-o", "-q", tempZip.Name())
	unzipCmd.Dir = rec.rootHostDir
	unzipCmd.Stdout = os.Stderr
	unzipCmd.Stderr = os.Stderr
	if err := unzipCmd.Run(); err != nil {
		return err
	}

	// TODO(someday): Stamp downloaded files.

	return nil

}
