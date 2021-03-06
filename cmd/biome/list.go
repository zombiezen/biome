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
	"time"

	"github.com/spf13/cobra"
	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

type listCommand struct {
	all   bool
	quiet bool
}

func newListCommand() *cobra.Command {
	c := new(listCommand)
	cmd := &cobra.Command{
		Use:           "list",
		Aliases:       []string{"ls"},
		Short:         "list created biomes",
		Args:          cobra.NoArgs,
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.run(cmd.Context())
		},
	}
	cmd.Flags().BoolVarP(&c.all, "all", "a", false, "show biomes in all directories")
	cmd.Flags().BoolVarP(&c.quiet, "quiet", "q", false, "only show IDs")
	return cmd
}

func (c *listCommand) run(ctx context.Context) (err error) {
	db, err := openDB(ctx)
	if err != nil {
		return err
	}
	defer db.Close()

	query := `select "id", "created_at", "root_host_dir" from "biomes" `
	var queryArgs []interface{}
	if !c.all {
		query += `where pathparentof("root_host_dir", ?) `
		currDir, err := os.Getwd()
		if err != nil {
			return err
		}
		queryArgs = append(queryArgs, currDir)
	}
	query += `order by "created_at" desc, "id";`
	err = sqlitex.Exec(db, query, func(stmt *sqlite.Stmt) error {
		id := stmt.ColumnText(0)
		createdAt, err := time.Parse(sqliteTimestampFormatMillis, stmt.ColumnText(1))
		if err != nil {
			return fmt.Errorf("biome[id=%q].created_at: %w", id, err)
		}
		rootHostDir := stmt.ColumnText(2)

		if c.quiet {
			_, err = fmt.Println(id)
		} else {
			_, err = fmt.Printf("%s\t%s\t%s\n", id, createdAt.Local().Format(time.RFC3339), rootHostDir)
		}
		return err
	}, queryArgs...)
	if err != nil {
		return err
	}
	return nil
}
