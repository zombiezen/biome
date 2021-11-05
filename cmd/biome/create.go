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
	"os"
	"time"

	"github.com/spf13/cobra"
	"zombiezen.com/go/log"
	"zombiezen.com/go/sqlite/sqlitex"
)

type createCommand struct {
}

func newCreateCommand() *cobra.Command {
	c := new(createCommand)
	cmd := &cobra.Command{
		Use:                   "create [options]",
		DisableFlagsInUseLine: true,
		Short:                 "create a new biome",
		Args:                  cobra.NoArgs,
		SilenceErrors:         true,
		SilenceUsage:          true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.run(cmd.Context())
		},
	}
	return cmd
}

func (c *createCommand) run(ctx context.Context) (err error) {
	now := time.Now()
	db, err := openDB(ctx)
	if err != nil {
		return err
	}
	defer db.Close()

	id, err := genHexDigits(16)
	if err != nil {
		return err
	}
	defer sqlitex.Save(db)(&err)
	err = sqlitex.Exec(db, `insert into "biomes" ("id", "created_at") values (?, ?);`, nil, id, now.UTC().Format(sqliteTimestampFormatMillis))
	if err != nil {
		return err
	}
	if dir, err := findBiomeDir(id); err != nil {
		log.Warnf(ctx, "%v", err)
	} else if err := os.MkdirAll(dir, 0o744); err != nil {
		log.Warnf(ctx, "%v", err)
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
