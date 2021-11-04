def install(biome, version, downloader, **_):
  golang_root = biome.path.join(biome.dirs.tools, "go")
  golang_dir = biome.path.join(golang_root, "go"+version)
  gopath_dir = biome.path.join(golang_root, "gopath")

  env = Environment(
    vars = {
      "GOROOT": golang_dir,
      "GOPATH": gopath_dir + ":" + biome.dirs.work,
    },
    prepend_path = [
      biome.path.join(gopath_dir, "bin"),
      biome.path.join(golang_dir, "bin"),
    ],
  )
  if biome.path.exists(golang_dir):
    return env

  download_url = "https://dl.google.com/go/go%s.%s-%s.tar.gz" % (version, biome.os, biome.arch)
  downloader.extract(biome, dst_dir=golang_dir, url=download_url, mode="strip")
  return env
