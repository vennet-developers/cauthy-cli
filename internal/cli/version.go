package cli

import (
	"flag"
	"fmt"
	"runtime"
	"runtime/debug"
)

// Set by the release build (-ldflags -X). A `go install` build has none and
// reads its module version instead.
var (
	Version = "dev"
	Commit  = ""
	Date    = ""
)

// Protocol is the auth-bff protocol version `cauthy conformance` certifies.
const Protocol = 1

const versionHelp = `cauthy version

Prints this build's version, commit and date, and the auth-bff protocol
version its conformance suite checks.
`

func init() { register("version", runVersion) }

func version() string {
	if Version != "dev" {
		return Version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return Version
}

func runVersion(args []string, env Env) int {
	fs := flag.NewFlagSet("version", flag.ContinueOnError)
	if code, done := parse(fs, args, env, versionHelp); done {
		return code
	}
	fmt.Fprintf(env.Out, "cauthy %s", version())
	if Commit != "" {
		short := Commit
		if len(short) > 12 {
			short = short[:12]
		}
		fmt.Fprintf(env.Out, " (%s, %s)", short, Date)
	}
	fmt.Fprintf(env.Out, "\nprotocol auth-bff %d\n%s %s/%s\n", Protocol, runtime.Version(), runtime.GOOS, runtime.GOARCH)
	return ExitOK
}
