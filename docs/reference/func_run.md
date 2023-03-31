## func run

Run a function locally

### Synopsis


NAME
	func run - Run a function locally

SYNOPSIS
	func run [-t|--container] [-v|--verbose] [-p|--path]

DESCRIPTION
	Run the function locally either on the host (default) or from within its
	container.

	Containerized Runs
	  The --container flag indicates that the function's container shuould be
	  run rather than running the source code directly.  This may require that
	  the funciton's container first be rebuilt, and is required if the function
	  has not been built yet:
	    $ func build && func run --container

	Process Scaffolding
	  When running a function, either within a container or on the host, the
	  function code is first wrapped in code which presents it as a process.
	  This "scaffolding" is transient, written for each build or run, and should
	  in most cases be transparent to a function author.  However, to customize,
	  or even completely replace this scafolding code, see the 'scaffold'
	  subcommand.

EXAMPLES

	o Run the function locally on the host with no containerization.
	  $ func run

	o Run the function locally from within its container.
	  func build && func run --container


```
func run
```

### Options

```
  -t, --container     Run the function in a container. ($FUNC_CONTAINER)
  -h, --help          help for run
  -p, --path string   Path to the function.  Default is current directory ($FUNC_PATH)
  -v, --verbose       Print verbose logs ($FUNC_VERBOSE)
```

### SEE ALSO

* [func](func.md)	 - func manages Knative Functions

