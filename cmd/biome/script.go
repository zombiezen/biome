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
	"sort"

	"github.com/spf13/cobra"
	"go.starlark.net/starlark"
	"zombiezen.com/go/biome"
)

type scriptCommand struct {
	home   string
	script string
}

func newScriptCommand() *cobra.Command {
	c := new(scriptCommand)
	cmd := &cobra.Command{
		Use:                   "script --home=DIR [options] SCRIPT",
		Short:                 "run an installer script",
		Args:                  cobra.ExactArgs(1),
		DisableFlagsInUseLine: true,
		SilenceErrors:         true,
		SilenceUsage:          true,
		RunE: func(cmd *cobra.Command, args []string) error {
			c.script = args[0]
			return c.run(cmd.Context())
		},
	}
	cmd.Flags().StringVar(&c.home, "home", "", "home directory to use inside biome")
	return cmd
}

func (c *scriptCommand) run(ctx context.Context) error {
	if c.home == "" {
		return fmt.Errorf("missing --home option")
	}
	bio := biome.Local{
		HomeDir: c.home,
	}
	var err error
	bio.WorkDir, err = os.Getwd()
	if err != nil {
		return err
	}
	thread := &starlark.Thread{}
	thread.SetLocal(threadContextKey, ctx)
	script, err := os.Open(c.script)
	if err != nil {
		return err
	}
	defer script.Close()
	predeclared := biomePredeclared(bio)
	if _, err := starlark.ExecFile(thread, c.script, script, predeclared); err != nil {
		return err
	}
	return nil
}

const threadContextKey = "zombiezen.com/go/biome.Context"

func threadContext(t *starlark.Thread) context.Context {
	ctx, _ := t.Local(threadContextKey).(context.Context)
	if ctx == nil {
		ctx = context.Background()
	}
	return ctx
}

func biomePredeclared(bio biome.Biome) starlark.StringDict {
	runBuiltin := func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
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
		if err := bio.Run(ctx, invocation); err != nil {
			return nil, err
		}
		return starlark.None, nil
	}
	return starlark.StringDict{
		"os":    starlark.String(bio.Describe().OS),
		"arch":  starlark.String(bio.Describe().Arch),
		"run":   starlark.NewBuiltin("run", runBuiltin),
		"dirs":  newDirsModule(bio.Dirs()),
		"paths": newPathsModule(bio),
	}
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

func newPathsModule(bio biome.Biome) *module {
	return &module{
		name: "paths",
		attrs: starlark.StringDict{
			"join": starlark.NewBuiltin("paths.join", func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
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
		},
	}
}

type module struct {
	name  string
	attrs starlark.StringDict
}

func (*module) Type() string          { return "module" }
func (*module) Freeze()               {}
func (*module) Truth() starlark.Bool  { return starlark.True }
func (*module) Hash() (uint32, error) { return 0, fmt.Errorf("module not hashable") }

func (mod *module) String() string { return "<module '" + mod.name + "'>" }

func (mod *module) Attr(name string) (starlark.Value, error) {
	return mod.attrs[name], nil
}

func (mod *module) Attrs() []string {
	attrs := make([]string, 0, len(mod.attrs))
	for k := range mod.attrs {
		attrs = append(attrs, k)
	}
	sort.Strings(attrs)
	return attrs
}
