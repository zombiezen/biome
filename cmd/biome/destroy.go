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
	"os"

	"github.com/spf13/cobra"
	"zombiezen.com/go/log"
	"zombiezen.com/go/sqlite/sqlitex"
)

type destroyCommand struct {
	id string
}

func newDestroyCommand() *cobra.Command {
	c := new(destroyCommand)
	cmd := &cobra.Command{
		Use:                   "destroy [options] ID",
		DisableFlagsInUseLine: true,
		Short:                 "destroy a biome",
		Args:                  cobra.ExactArgs(1),
		SilenceErrors:         true,
		SilenceUsage:          true,
		RunE: func(cmd *cobra.Command, args []string) error {
			c.id = args[0]
			return c.run(cmd.Context())
		},
	}
	return cmd
}

func (c *destroyCommand) run(ctx context.Context) (err error) {
	db, err := openDB(ctx)
	if err != nil {
		return err
	}
	defer db.Close()

	defer sqlitex.Save(db)(&err)
	if err := verifyBiomeExists(db, c.id); err != nil {
		return fmt.Errorf("destroy %q: %v", c.id, err)
	}
	err = sqlitex.Exec(db, `delete from "biomes" where "id" = ?;`, nil, c.id)
	if err != nil {
		return fmt.Errorf("destroy %q: %v", c.id, err)
	}

	// TODO(soon): Delete write-protected files.
	if dir, err := findBiomeDir(c.id); err != nil {
		log.Warnf(ctx, "Cleaning up biome: %v", err)
	} else if err := os.RemoveAll(dir); err != nil {
		log.Warnf(ctx, "Cleaning up biome: %v", err)
	}
	return nil
}
