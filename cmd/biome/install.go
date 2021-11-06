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
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yourbase/commons/ini"
	"go.starlark.net/starlark"
	"go4.org/xdgdir"
	"zombiezen.com/go/biome"
	"zombiezen.com/go/biome/downloader"
	"zombiezen.com/go/biome/internal/extract"
	"zombiezen.com/go/sqlite/sqlitex"
)

type installCommand struct {
	biomeID string
	script  string
	version string
}

func newInstallCommand() *cobra.Command {
	c := new(installCommand)
	cmd := &cobra.Command{
		Use:                   "install [options] SCRIPT VERSION",
		DisableFlagsInUseLine: true,
		Short:                 "run an installer script",
		Args:                  cobra.ExactArgs(2),
		SilenceErrors:         true,
		SilenceUsage:          true,
		RunE: func(cmd *cobra.Command, args []string) error {
			c.script = args[0]
			c.version = args[1]
			return c.run(cmd.Context())
		},
	}
	cmd.Flags().StringVarP(&c.biomeID, "biome", "b", "", "biome to run inside")
	return cmd
}

func (c *installCommand) run(ctx context.Context) (err error) {
	db, err := openDB(ctx)
	if err != nil {
		return err
	}
	defer db.Close()
	defer sqlitex.Save(db)(&err)
	biomeID, rootHostDir, err := findBiome(db, c.biomeID)
	if err != nil {
		return err
	}
	env, err := readBiomeEnvironment(db, biomeID)
	if err != nil {
		return err
	}

	biomeRoot, err := computeBiomeRoot(biomeID)
	if err != nil {
		return err
	}
	bio := setupBiome(biomeRoot, rootHostDir)
	thread := &starlark.Thread{}
	thread.SetLocal(threadContextKey, ctx)
	script, err := os.Open(c.script)
	if err != nil {
		return err
	}
	defer script.Close()
	predeclared := starlark.StringDict{
		"Environment": starlark.NewBuiltin("Environment", builtinEnvironmentCtor),
	}
	globals, err := starlark.ExecFile(thread, c.script, script, predeclared)
	if err != nil {
		return err
	}
	installFuncValue := globals["install"]
	if installFuncValue == nil {
		return fmt.Errorf("no install function found")
	}
	installFunc, ok := installFuncValue.(*starlark.Function)
	if !ok {
		return fmt.Errorf("`install` is declared as %s instead of function", installFuncValue.Type())
	}
	if !installFunc.HasKwargs() {
		//lint:ignore ST1005 referencing Environment constructor
		return fmt.Errorf("install function does not permit extra keyword arguments. " +
			"Please add `**kwargs` to the end of install's parameters for forward compatibility.")
	}
	cachePath := xdgdir.Cache.Path()
	if cachePath == "" {
		return fmt.Errorf("%v not set", xdgdir.Cache)
	}
	myDownloader := downloader.New(filepath.Join(cachePath, cacheSubdirName, "downloads"))
	installReturnValue, err := starlark.Call(
		thread,
		installFunc,
		starlark.Tuple{biomeValue(bio), starlark.String(c.version)},
		[]starlark.Tuple{
			{starlark.String("downloader"), downloaderValue(myDownloader)},
		},
	)
	if err != nil {
		return err
	}

	installReturn, ok := installReturnValue.(*envValue)
	if !ok {
		return fmt.Errorf("`install` returned a %s instead of Environment", installReturnValue.Type())
	}
	installEnv, err := installReturn.toEnvironment()
	if err != nil {
		return fmt.Errorf("install return value: %w", err)
	}
	if err := writeBiomeEnvironment(db, biomeID, env.Merge(installEnv)); err != nil {
		return err
	}
	return nil
}

func toEnvFile(e biome.Environment) []byte {
	if e.IsEmpty() {
		return nil
	}
	f := new(ini.File)
	keys := make([]string, 0, len(e.Vars)+1)
	for k := range e.Vars {
		keys = append(keys, k)
	}
	const pathVar = "PATH"
	if e.Vars[pathVar] == "" && len(e.PrependPath)+len(e.AppendPath) > 0 {
		keys = append(keys, pathVar)
	}
	sort.Strings(keys)
	for _, k := range keys {
		var v string
		if k == pathVar {
			parts := make([]string, 0, len(e.PrependPath)+len(e.AppendPath)+1)
			parts = append(parts, e.PrependPath...)
			if p := e.Vars[pathVar]; p != "" {
				parts = append(parts, p)
			}
			parts = append(parts, e.AppendPath...)
			// TODO(windows): List separator is not always ':'.
			v = strings.Join(parts, ":")
		} else {
			v = e.Vars[k]
		}
		f.Set("", k, v)
	}
	data, err := f.MarshalText()
	if err != nil {
		panic(err)
	}
	return data
}

const threadContextKey = "zombiezen.com/go/biome.Context"

func threadContext(t *starlark.Thread) context.Context {
	ctx, _ := t.Local(threadContextKey).(context.Context)
	if ctx == nil {
		ctx = context.Background()
	}
	return ctx
}

type envValue struct {
	vars        *starlark.Dict
	prependPath *starlark.List
	appendPath  *starlark.List
}

func builtinEnvironmentCtor(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	ev := new(envValue)
	err := starlark.UnpackArgs(fn.Name(), args, kwargs,
		"vars?", &ev.vars,
		"prepend_path?", &ev.prependPath,
		"append_path?", &ev.appendPath,
	)
	if err != nil {
		return nil, err
	}
	if ev.vars == nil {
		ev.vars = new(starlark.Dict)
	}
	if ev.prependPath == nil {
		ev.prependPath = new(starlark.List)
	}
	if ev.appendPath == nil {
		ev.appendPath = new(starlark.List)
	}
	return ev, nil
}

func (ev *envValue) String() string {
	return fmt.Sprintf("Environment(vars=%v, prepend_path=%v, append_path=%v)",
		ev.vars, ev.prependPath, ev.appendPath)
}

func (ev *envValue) Type() string {
	return "Environment"
}

func (ev *envValue) Freeze() {
	ev.vars.Freeze()
	ev.prependPath.Freeze()
	ev.appendPath.Freeze()
}

func (ev *envValue) Truth() starlark.Bool {
	return ev.vars.Len() > 0 || ev.prependPath.Len() > 0 || ev.appendPath.Len() > 0
}

func (ev *envValue) Hash() (uint32, error) {
	//lint:ignore ST1005 referencing Environment constructor
	return 0, fmt.Errorf("Environment not hashable")
}

func (ev *envValue) Attr(name string) (starlark.Value, error) {
	switch name {
	case "vars":
		return ev.vars, nil
	case "prepend_path":
		return ev.prependPath, nil
	case "append_path":
		return ev.appendPath, nil
	default:
		return nil, nil
	}
}

func (ev *envValue) AttrNames() []string {
	return []string{
		"append_path",
		"prepend_path",
		"vars",
	}
}

func (ev *envValue) toEnvironment() (biome.Environment, error) {
	var e biome.Environment
	if n := ev.vars.Len(); n > 0 {
		e.Vars = make(map[string]string, n)
		for _, kv := range ev.vars.Items() {
			k, ok := starlark.AsString(kv[0])
			if !ok {
				return biome.Environment{}, fmt.Errorf("invalid Environment.vars key %v", kv[0])
			}
			v, ok := starlark.AsString(kv[1])
			if !ok {
				return biome.Environment{}, fmt.Errorf("invalid Environment.vars value %v for key %q", kv[1], k)
			}
			e.Vars[k] = v
		}
	}
	for i, n := 0, ev.appendPath.Len(); i < n; i++ {
		pv := ev.appendPath.Index(i)
		p, ok := starlark.AsString(pv)
		if !ok {
			return biome.Environment{}, fmt.Errorf("invalid Environment.appendPath[%d] value %v", i, pv)
		}
		e.AppendPath = append(e.AppendPath, p)
	}
	for i, n := 0, ev.prependPath.Len(); i < n; i++ {
		pv := ev.prependPath.Index(i)
		p, ok := starlark.AsString(pv)
		if !ok {
			return biome.Environment{}, fmt.Errorf("invalid Environment.prependPath[%d] value %v", i, pv)
		}
		e.PrependPath = append(e.PrependPath, p)
	}
	return e, nil
}

type biomeWrapper struct {
	biome biome.Biome
	attrs starlark.StringDict
}

func biomeValue(bio biome.Biome) *biomeWrapper {
	bw := &biomeWrapper{biome: bio}
	bw.attrs = starlark.StringDict{
		"os":   starlark.String(bio.Describe().OS),
		"arch": starlark.String(bio.Describe().Arch),
		"run":  starlark.NewBuiltin("run", bw.runBuiltin),
		"dirs": newDirsModule(bio.Dirs()),
		"path": newPathModule(bio),
	}
	return bw
}

func (*biomeWrapper) Type() string          { return "biome" }
func (*biomeWrapper) Freeze()               {}
func (*biomeWrapper) Truth() starlark.Bool  { return starlark.True }
func (*biomeWrapper) Hash() (uint32, error) { return 0, fmt.Errorf("biome not hashable") }
func (*biomeWrapper) String() string        { return "<biome>" }

func (bw *biomeWrapper) Attr(name string) (starlark.Value, error) {
	return bw.attrs[name], nil
}

func (bw *biomeWrapper) AttrNames() []string {
	return sortedStringDictKeys(bw.attrs)
}

func (bw *biomeWrapper) runBuiltin(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	ctx := threadContext(thread)
	var argv *starlark.List
	invocation := &biome.Invocation{
		Stdout: os.Stderr,
		Stderr: os.Stderr,
	}
	err := starlark.UnpackArgs(fn.Name(), args, kwargs,
		"argv", &argv,
		"dir??", &invocation.Dir,
	)
	if err != nil {
		return nil, err
	}
	invocation.Argv = make([]string, 0, argv.Len())
	for i := 0; i < argv.Len(); i++ {
		arg, ok := starlark.AsString(argv.Index(i))
		if !ok {
			return nil, fmt.Errorf("run: could not convert argv[%d] to string", i)
		}
		invocation.Argv = append(invocation.Argv, arg)
	}
	if err := bw.biome.Run(ctx, invocation); err != nil {
		return nil, err
	}
	return starlark.None, nil
}

func newDirsModule(dirs *biome.Dirs) *module {
	return &module{
		name: "dirs",
		attrs: starlark.StringDict{
			"work":  starlark.String(dirs.Work),
			"home":  starlark.String(dirs.Home),
			"tools": starlark.String(dirs.Tools),
		},
	}
}

func newPathModule(bio biome.Biome) *module {
	return &module{
		name: "path",
		attrs: starlark.StringDict{
			"join": starlark.NewBuiltin("path.join", func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
				if len(kwargs) != 0 {
					return nil, fmt.Errorf("%s: keyword arguments not allowed", fn.Name())
				}
				stringArgs := make([]string, 0, args.Len())
				for i := 0; i < args.Len(); i++ {
					arg, ok := starlark.AsString(args.Index(i))
					if !ok {
						return nil, fmt.Errorf("%s: could not convert arg[%d] to string", fn.Name(), i)
					}
					stringArgs = append(stringArgs, arg)
				}
				return starlark.String(bio.JoinPath(stringArgs...)), nil
			}),
			"exists": starlark.NewBuiltin("path.exists", func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
				var path string
				if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "path", &path); err != nil {
					return nil, err
				}
				_, err := biome.EvalSymlinks(threadContext(thread), bio, path)
				return starlark.Bool(err == nil), nil
			}),
		},
	}
}

func downloaderValue(d *downloader.Downloader) *module {
	return &module{
		name: "downloader",
		attrs: starlark.StringDict{
			"extract": starlark.NewBuiltin("extract", func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
				opts := &extract.Options{
					Downloader: d,
					Output:     os.Stderr,
				}
				var bw *biomeWrapper
				mode := "tarbomb"
				err := starlark.UnpackArgs(fn.Name(), args, kwargs,
					"biome", &bw,
					"dst_dir", &opts.DestinationDir,
					"url", &opts.URL,
					"mode?", &mode,
				)
				if err != nil {
					return nil, err
				}
				opts.Biome = bw.biome
				switch mode {
				case "tarbomb":
					opts.ExtractMode = extract.Tarbomb
				case "strip":
					opts.ExtractMode = extract.StripTopDirectory
				default:
					return nil, fmt.Errorf("%s: invalid mode %q", fn.Name(), mode)
				}
				if err := extract.Extract(threadContext(thread), opts); err != nil {
					return nil, err
				}
				return starlark.None, nil
			}),
		},
	}
}

var _ starlark.HasAttrs = (*module)(nil)

type module struct {
	name  string
	attrs starlark.StringDict
}

func (*module) Type() string          { return "module" }
func (*module) Freeze()               {}
func (*module) Truth() starlark.Bool  { return starlark.True }
func (*module) Hash() (uint32, error) { return 0, fmt.Errorf("module not hashable") }
func (mod *module) String() string    { return "<module '" + mod.name + "'>" }

func (mod *module) Attr(name string) (starlark.Value, error) {
	return mod.attrs[name], nil
}

func (mod *module) AttrNames() []string {
	return sortedStringDictKeys(mod.attrs)
}

func sortedStringDictKeys(d starlark.StringDict) []string {
	keys := make([]string, 0, len(d))
	for k := range d {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
