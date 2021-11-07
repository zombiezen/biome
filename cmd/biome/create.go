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
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"zombiezen.com/go/sqlite/sqlitex"
)

type createCommand struct {
	rootDir string
}

func newCreateCommand() *cobra.Command {
	c := new(createCommand)
	cmd := &cobra.Command{
		Use:           "create",
		Short:         "create a new biome",
		Args:          cobra.NoArgs,
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.run(cmd.Context())
		},
	}
	cmd.Flags().StringVar(&c.rootDir, "root", ".", "root of the directory to copy into the biome")
	return cmd
}

func (c *createCommand) run(ctx context.Context) (err error) {
	now := time.Now()
	rootDir, err := filepath.Abs(c.rootDir)
	if err != nil {
		return err
	}
	db, err := openDB(ctx)
	if err != nil {
		return err
	}
	defer db.Close()

	id, err := genHexDigits(16)
	if err != nil {
		return err
	}

	endFn, err := sqlitex.ImmediateTransaction(db)
	if err != nil {
		return err
	}
	defer endFn(&err)
	err = sqlitex.Exec(db, `insert into "biomes" ("id", "created_at", "root_host_dir") values (?, ?, ?);`, nil,
		id, now.UTC().Format(sqliteTimestampFormatMillis), rootDir)
	if err != nil {
		return err
	}
	rec := &biomeRecord{
		id:          id,
		rootHostDir: rootDir,
	}
	rec.supportRoot, err = computeSupportRoot(id)
	if err != nil {
		return err
	}
	if _, err := rec.setup(ctx, db); err != nil {
		return err
	}
	fmt.Println(id)
	return nil
}

func genHexDigits(nbytes int) (string, error) {
	bits := make([]byte, nbytes)
	if _, err := rand.Read(bits); err != nil {
		return "", fmt.Errorf("generate id: %w", err)
	}
	return hex.EncodeToString(bits), nil
}
