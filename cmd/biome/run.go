// Copyright 2021 Ross Light
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
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
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"golang.org/x/term"
	"zombiezen.com/go/biome"
	"zombiezen.com/go/sqlite/sqlitex"
)

type runCommand struct {
	biomeID string
	argv    []string
}

func newRunCommand() *cobra.Command {
	c := new(runCommand)
	cmd := &cobra.Command{
		Use:                   "run [options] --biome=ID PROGRAM [ARG [...]]",
		DisableFlagsInUseLine: true,
		Short:                 "run a command inside a biome",
		Args:                  cobra.MinimumNArgs(1),
		SilenceErrors:         true,
		SilenceUsage:          true,
		RunE: func(cmd *cobra.Command, args []string) error {
			c.argv = args
			return c.run(cmd.Context())
		},
	}
	cmd.Flags().StringVarP(&c.biomeID, "biome", "b", "", "biome to run inside")
	return cmd
}

func (c *runCommand) run(ctx context.Context) error {
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

	currDir, err := os.Getwd()
	if err != nil {
		return err
	}
	relDir, err := filepath.Rel(rec.rootHostDir, currDir)
	if err != nil {
		return err
	}
	if !isSubFilepath(relDir) {
		relDir = ""
	}

	// TODO(soon): Exit with same exit code.
	return bio.Run(ctx, &biome.Invocation{
		Argv:        c.argv,
		Dir:         relDir,
		Stdin:       os.Stdin,
		Stdout:      os.Stdout,
		Stderr:      os.Stderr,
		Interactive: term.IsTerminal(int(os.Stdin.Fd())),
	})
}
