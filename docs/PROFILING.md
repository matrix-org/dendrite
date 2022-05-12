---
title: Profiling
parent: Development
permalink: /development/profiling
---

# Profiling Dendrite

If you are running into problems with Dendrite using excessive resources (e.g. CPU or RAM) then you can use the profiler to work out what is happening.

Dendrite contains an embedded profiler called `pprof`, which is a part of the standard Go toolchain.

## Enable the profiler

To enable the profiler, start Dendrite with the `PPROFLISTEN` environment variable. This variable specifies which address and port to listen on, e.g.

```
PPROFLISTEN=localhost:65432 ./bin/dendrite-monolith-server ...
```

If pprof has been enabled successfully, a log line at startup will show that pprof is listening:

```
WARN[2020-12-03T13:32:33.669405000Z] [/Users/neilalexander/Desktop/dendrite/internal/log.go:87] SetupPprof
  Starting pprof on localhost:65432
```

All examples from this point forward assume `PPROFLISTEN=localhost:65432` but you may need to adjust as necessary for your setup.

## Profiling CPU usage

To examine where CPU time is going, you can call the `profile` endpoint:

```
http://localhost:65432/debug/pprof/profile?seconds=30
```

The profile will run for the specified number of `seconds` and then will produce a result.

### Examine a profile using the Go toolchain

If you have Go installed and want to explore the profile, you can invoke `go tool pprof` to start the profile directly. The `-http=` parameter will instruct `go tool pprof` to start a web server providing a view of the captured profile:

```
go tool pprof -http=localhost:23456 http://localhost:65432/debug/pprof/profile?seconds=30
```

You can then visit `http://localhost:23456` in your web browser to see a visual representation of the profile. Particularly usefully, in the "View" menu, you can select "Flame Graph" to see a proportional interactive graph of CPU usage.

### Download a profile to send to someone else

If you don't have the Go tools installed but just want to capture the profile to send to someone else, you can instead use `curl` to download the profiler results:

```
curl -O http://localhost:65432/debug/pprof/profile?seconds=30
```

This will block for the specified number of seconds, capturing information about what Dendrite is doing, and then produces a `profile` file, which you can send onward.

## Profiling memory usage

To examine where memory usage is going, you can call the `heap` endpoint:

```
http://localhost:65432/debug/pprof/heap
```

The profile will return almost instantly.

### Examine a profile using the Go toolchain

If you have Go installed and want to explore the profile, you can invoke `go tool pprof` to start the profile directly. The `-http=` parameter will instruct `go tool pprof` to start a web server providing a view of the captured profile:

```
go tool pprof -http=localhost:23456 http://localhost:65432/debug/pprof/heap
```

You can then visit `http://localhost:23456` in your web browser to see a visual representation of the profile. The "Sample" menu lets you select between four different memory profiles:

* `inuse_space`: Shows how much actual heap memory is allocated per function (this is generally the most useful profile when diagnosing high memory usage)
* `inuse_objects`: Shows how many heap objects are allocated per function
* `alloc_space`: Shows how much memory has been allocated per function (although that memory may have since been deallocated)
* `alloc_objects`: Shows how many allocations have been made per function (although that memory may have since been deallocated)

Also in the "View" menu, you can select "Flame Graph" to see a proportional interactive graph of the memory usage.

### Download a profile to send to someone else

If you don't have the Go tools installed but just want to capture the profile to send to someone else, you can instead use `curl` to download the profiler results:

```
curl -O http://localhost:65432/debug/pprof/heap
```

This will almost instantly produce a `heap` file, which you can send onward.
