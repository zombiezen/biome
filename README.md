# biome: Lightweight, Reproducible Dev Environments

biome is a CLI for running and installing programs under controlled conditions,
without having to resort to heavyweight solutions like Docker. There's no
daemon and no need for root access.

Example:

```shell
$ biome create
9532df19058e937b8d5bf0e0c5814f4f
$ biome install go.star 1.17.3
$ biome run -- go version
go version go1.17.3 linux/amd64
```

## Getting Started

To create a biome, run:

```shell
mkdir biome-test &&
cd biome-test &&
biome create
```

Biomes are associated with a directory: everything inside that directory
will be available to use in the biome. To run a command inside the biome, use
`biome run`:

```shell
echo "Hello, World" > foo.txt &&
biome run -- cat foo.txt
```

One caveat: each biome has its own replica of the directory it is associated
with. If a file gets created or modified in the biome, then it needs to be
explicitly pulled down into the source directory.

```shell
biome run -- cp foo.txt bar.txt &&
biome pull bar.txt
```

Biomes also support installing software with [Starlark][] scripts:

```shell
curl -fsSLO https://github.com/zombiezen/biome/raw/main/installers/go.star &&
biome install go.star 1.17.3 &&
biome run -- go version
```

Once you're done with a biome, you can reclaim disk space with `biome destroy`:

```shell
biome destroy
```

[Starlark]: https://github.com/bazelbuild/starlark
