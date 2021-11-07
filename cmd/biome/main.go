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
	"embed"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"
	"go4.org/xdgdir"
	"golang.org/x/sys/unix"
	"zombiezen.com/go/biome"
	"zombiezen.com/go/log"
	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitemigration"
	"zombiezen.com/go/sqlite/sqlitex"
)

const cacheSubdirName = "zombiezen-biome"

const sqliteTimestampFormatMillis = "2006-01-02 15:04:05.999"

func main() {
	root := &cobra.Command{
		Use:           "biome",
		Short:         "Lightweight dev environments",
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	debug := root.PersistentFlags().Bool("debug", false, "show debug logs")
	root.PersistentPreRun = func(cmd *cobra.Command, args []string) {
		ensureLogger(*debug)
	}
	root.AddCommand(
		newCreateCommand(),
		newDestroyCommand(),
		newInstallCommand(),
		newListCommand(),
		newRunCommand(),
	)

	ctx, cancel := signal.NotifyContext(context.Background(), unix.SIGTERM, unix.SIGINT)
	err := root.ExecuteContext(ctx)
	cancel()
	if err != nil {
		ensureLogger(false)
		log.Errorf(ctx, "%v", err)
		os.Exit(1)
	}
}

var logInit sync.Once

func ensureLogger(debug bool) {
	logInit.Do(func() {
		level := log.Info
		if debug {
			level = log.Debug
		}
		log.SetDefault(&log.LevelFilter{
			Min:    level,
			Output: log.New(os.Stderr, "biome: ", 0, nil),
		})
	})
}

func openDB(ctx context.Context) (*sqlite.Conn, error) {
	cacheDir := xdgdir.Cache.Path()
	if cacheDir == "" {
		return nil, fmt.Errorf("open database: %v not defined", xdgdir.Cache)
	}
	dbPath := filepath.Join(cacheDir, cacheSubdirName, "biomes.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o744); err != nil {
		return nil, fmt.Errorf("open database: %v", err)
	}
	conn, err := sqlite.OpenConn(dbPath, sqlite.OpenCreate, sqlite.OpenReadWrite, sqlite.OpenWAL, sqlite.OpenNoMutex)
	if err != nil {
		return nil, fmt.Errorf("open database: %v", err)
	}
	conn.SetInterrupt(ctx.Done())
	conn.SetBusyTimeout(60 * time.Second) // TODO(someday): Block until interrupt.
	if err := sqlitex.ExecTransient(conn, "PRAGMA foreign_keys = on;", nil); err != nil {
		conn.Close()
		return nil, fmt.Errorf("open database: %v", err)
	}
	err = conn.CreateFunction("regexp", &sqlite.FunctionImpl{
		NArgs:         2,
		Deterministic: true,
		AllowIndirect: true,
		Scalar: func(ctx sqlite.Context, args []sqlite.Value) (sqlite.Value, error) {
			// First: attempt to retrieve the compiled regexp from a previous call.
			re, ok := ctx.AuxData(0).(*regexp.Regexp)
			if !ok {
				// Auxiliary data not present. Either this is the first call with this
				// argument, or SQLite has discarded the auxiliary data.
				var err error
				re, err = regexp.Compile(args[0].Text())
				if err != nil {
					return sqlite.Value{}, fmt.Errorf("regexp: %w", err)
				}
				// Store the auxiliary data for future calls.
				ctx.SetAuxData(0, re)
			}

			found := 0
			if re.MatchString(args[1].Text()) {
				found = 1
			}
			return sqlite.IntegerValue(int64(found)), nil
		},
	})
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("open database: %v", err)
	}
	err = conn.CreateFunction("pathparentof", &sqlite.FunctionImpl{
		NArgs:         2,
		Deterministic: true,
		Scalar: func(ctx sqlite.Context, args []sqlite.Value) (sqlite.Value, error) {
			if args[0].Type() == sqlite.TypeNull || args[1].Type() == sqlite.TypeNull {
				return sqlite.Value{}, nil
			}
			parent := filepath.Clean(args[0].Text())
			child := filepath.Clean(args[1].Text())
			if parent == child || strings.HasPrefix(child, parent+string(filepath.Separator)) {
				return sqlite.IntegerValue(1), nil
			} else {
				return sqlite.IntegerValue(0), nil
			}
		},
	})
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("open database: %v", err)
	}
	if err := sqlitemigration.Migrate(ctx, conn, loadSchema()); err != nil {
		conn.Close()
		return nil, fmt.Errorf("open database: %v", err)
	}
	return conn, nil
}

//go:embed dbschema/*.sql
var schemaFiles embed.FS

func loadSchema() sqlitemigration.Schema {
	schema := sqlitemigration.Schema{AppID: 0x604662be}
	for i := 1; ; i++ {
		migration, err := schemaFiles.ReadFile(fmt.Sprintf("dbschema/%02d.sql", i))
		if errors.Is(err, os.ErrNotExist) {
			break
		}
		if err != nil {
			panic(err)
		}
		schema.Migrations = append(schema.Migrations, string(migration))
	}
	return schema
}

type biomeRecord struct {
	id          string
	rootHostDir string
	supportRoot string
}

// findBiome fetches the biome record for an ID reference or the empty string.
func findBiome(conn *sqlite.Conn, arg string) (*biomeRecord, error) {
	if arg == "" {
		currDir, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		const query = `select "id", "root_host_dir" from "biomes" where pathparentof("root_host_dir", ?) limit 2;`
		n := 0
		rec := new(biomeRecord)
		err = sqlitex.Exec(conn, query, func(stmt *sqlite.Stmt) error {
			n++
			rec.id = stmt.ColumnText(0)
			rec.rootHostDir = stmt.ColumnText(1)
			return nil
		}, currDir)
		if err != nil {
			return nil, err
		}
		if n == 0 {
			return nil, fmt.Errorf("no biomes in %s", currDir)
		}
		if n > 1 {
			return nil, fmt.Errorf("multiple biomes in %s; use --biome=ID to disambiguate", currDir)
		}
		rec.supportRoot, err = computeSupportRoot(rec.id)
		if err != nil {
			return nil, err
		}
		return rec, nil
	}
	// TODO(soon): Allow prefix of ID.
	const query = `select "id", "root_host_dir" from "biomes" where "id" = ? limit 1;`
	var rec *biomeRecord
	err := sqlitex.Exec(conn, query, func(stmt *sqlite.Stmt) error {
		rec = &biomeRecord{
			id:          stmt.ColumnText(0),
			rootHostDir: stmt.ColumnText(1),
		}
		return nil
	}, arg)
	if err != nil {
		return nil, err
	}
	if rec == nil {
		return nil, fmt.Errorf("no biome with ID %q", arg)
	}
	rec.supportRoot, err = computeSupportRoot(rec.id)
	if err != nil {
		return nil, err
	}
	return rec, nil
}

func (rec *biomeRecord) setup(ctx context.Context, conn *sqlite.Conn) (biome.Biome, error) {
	bio := biome.Local{
		HomeDir: filepath.Join(rec.supportRoot, "home"),
		WorkDir: filepath.Join(rec.supportRoot, "work"),
	}
	if err := os.MkdirAll(bio.HomeDir, 0o744); err != nil {
		return nil, fmt.Errorf("open biome %s: %v", rec.id, err)
	}
	if err := os.MkdirAll(bio.WorkDir, 0o744); err != nil {
		return nil, fmt.Errorf("open biome %s: %v", rec.id, err)
	}
	if err := pushWorkDir(ctx, conn, rec, bio); err != nil {
		return nil, err
	}
	return bio, nil
}

// computeSupportRoot returns the cache directory that contains the biome's
// supporting files.
func computeSupportRoot(id string) (string, error) {
	if len(id) <= 2 {
		return "", fmt.Errorf("locate biome directory: id %q too short", id)
	}
	cacheDir := xdgdir.Cache.Path()
	if cacheDir == "" {
		return "", fmt.Errorf("locate biome directory: %v not defined", xdgdir.Cache)
	}
	bdir, err := filepath.Abs(filepath.Join(cacheDir, cacheSubdirName, "biomes", id[:2], id[2:]))
	if err != nil {
		return "", fmt.Errorf("locate biome directory: %v", err)
	}
	return bdir, nil
}

func readBiomeEnvironment(conn *sqlite.Conn, id string) (e biome.Environment, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("read biome %q environment: %w", id, err)
		}
	}()
	defer sqlitex.Save(conn)(&err)

	const varQuery = `select "name", "value" from "env_vars" where "biome_id" = ?;`
	e = biome.Environment{}
	err = sqlitex.ExecTransient(conn, varQuery, func(stmt *sqlite.Stmt) error {
		if e.Vars == nil {
			e.Vars = make(map[string]string)
		}
		e.Vars[stmt.ColumnText(0)] = stmt.ColumnText(1)
		return nil
	}, id)
	if err != nil {
		return biome.Environment{}, err
	}

	pathPartsQuery := conn.Prep(`select "directory" from "path_parts" ` +
		`where "biome_id" = :biome_id and "position" = :position ` +
		`order by "index" asc;`)
	pathPartsQuery.SetText(":biome_id", id)
	pathPartsQuery.SetText(":position", "prepend")
	for {
		hasRow, err := pathPartsQuery.Step()
		if err != nil {
			return biome.Environment{}, err
		}
		if !hasRow {
			break
		}
		e.PrependPath = append(e.PrependPath, pathPartsQuery.ColumnText(0))
	}
	if err := pathPartsQuery.Reset(); err != nil {
		return biome.Environment{}, err
	}

	pathPartsQuery.SetText(":position", "append")
	for {
		hasRow, err := pathPartsQuery.Step()
		if err != nil {
			return biome.Environment{}, err
		}
		if !hasRow {
			break
		}
		e.AppendPath = append(e.AppendPath, pathPartsQuery.ColumnText(0))
	}
	if err := pathPartsQuery.Reset(); err != nil {
		return biome.Environment{}, err
	}

	return e, nil
}

func writeBiomeEnvironment(conn *sqlite.Conn, id string, e biome.Environment) (err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("write biome %q environment: %w", id, err)
		}
	}()
	defer sqlitex.Save(conn)(&err)

	err = sqlitex.ExecTransient(conn, `delete from "env_vars" where "biome_id" = ?;`, nil, id)
	if err != nil {
		return err
	}
	err = sqlitex.ExecTransient(conn, `delete from "path_parts" where "biome_id" = ?;`, nil, id)
	if err != nil {
		return err
	}

	insertVarStmt := conn.Prep(`insert into "env_vars" ("biome_id", "name", "value") values (?, ?, ?);`)
	insertVarStmt.BindText(1, id)
	for k, v := range e.Vars {
		insertVarStmt.BindText(2, k)
		insertVarStmt.BindText(3, v)
		if _, err := insertVarStmt.Step(); err != nil {
			return fmt.Errorf("set %s: %w", k, err)
		}
		if err := insertVarStmt.Reset(); err != nil {
			return fmt.Errorf("set %s: %w", k, err)
		}
	}

	insertPathPartStmt := conn.Prep(`insert into "path_parts" ("biome_id", "position", "index", "directory") values (?, ?, ?, ?);`)
	insertPathParts := func(parts []string) error {
		for i, dir := range parts {
			insertPathPartStmt.BindInt64(3, int64(i))
			insertPathPartStmt.BindText(4, dir)
			if _, err := insertPathPartStmt.Step(); err != nil {
				return err
			}
			if err := insertPathPartStmt.Reset(); err != nil {
				return err
			}
		}
		return nil
	}

	insertPathPartStmt.BindText(1, id)
	insertPathPartStmt.BindText(2, "prepend")
	if err := insertPathParts(e.PrependPath); err != nil {
		return err
	}
	insertPathPartStmt.BindText(3, "append")
	if err := insertPathParts(e.AppendPath); err != nil {
		return err
	}

	return nil
}
