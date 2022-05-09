# Building Dendrite

## Prerequisites

The following tools make up the minimum toolchain for building the
Dendrite binaries:

* Go 1.16 or higher
* Gcc

### Windows fast path

If you want to build Dendrite on Windows:

* `go` can be downloaded from [Go Downloads](https://go.dev/dl/);
* `gcc` can be downloaded from [MinGW-w64](https://www.mingw-w64.org/);

Alternatively, if you use [Chocolatey](https://chocolatey.org/),
you can install *Go* and *Gcc* with the following command:

    choco install golang mingw -y

## How to obtain the source code

There are basically two ways of obtaining the Dendrite source code:

* **clone** the [official Dendrite Git repository on GitHub](https://github.com/matrix-org/dendrite)
  ```bash
  git clone https://github.com/matrix-org/dendrite.git
  cd dendrite
  ```
* **download** the archived source code of a specific release from the
  official [release page](https://github.com/matrix-org/dendrite/releases);
  the source code is provided in two different archives named - for
  example, for version 0.8.2 - respectively `v0.8.2.zip` (Windows) and
  `v0.8.2.tar.gz` (Linux and UNIX-like systems): just download the one
  suitable for your development platform;

Cloning the repository is of course suggested, if you intend to make
local changes to the code, or if you simply intend to follow the
evolution of Dendrite and test the changes before a new official version
is released.

## How to build Dendrite

However you got the Dendrite sources, to build the binaries, you have to
enter the `dendrite` directory before starting.

### On Linux, or UNIX-like systems

```bash
cd dendrite
./build.sh
```

### On Windows

```dos
cd dendrite
build.cmd
```

If the build process ends successfully, the binary files can be found in
the `bin` directory inside the starting `dendrite` directory.

From now on, you can start [installing or updating Dendrite](INSTALL.md)
on the server of your choice.
