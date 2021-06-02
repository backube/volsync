package main

import (
	goflag "flag"
	"os"

	scribecmd "github.com/backube/scribe/pkg/cmd"

	"github.com/spf13/pflag"
	"k8s.io/component-base/logs"
	"k8s.io/klog/v2"

	_ "k8s.io/client-go/plugin/pkg/client/auth"
)

var (
	scribeVersion = "0.0.0"
)

// this function is copied from oc/cmd/oc/oc.go
func injectLoglevelFlag(flags *pflag.FlagSet) {
	from := goflag.CommandLine
	if flag := from.Lookup("v"); flag != nil {
		if level, ok := flag.Value.(*klog.Level); ok {
			levelPtr := (*int32)(level)
			flags.Int32Var(levelPtr, "loglevel", 0, "Set the level of log output (0-10)")
		}
	}
}

func main() {
	logs.InitLogs()
	defer logs.FlushLogs()
	flags := pflag.NewFlagSet("kubectl-scribe", pflag.ExitOnError)
	pflag.CommandLine = flags
	injectLoglevelFlag(pflag.CommandLine)
	scribeCmd := scribecmd.NewCmdScribe(os.Stdin, os.Stdout, os.Stderr)

	scribeCmd.Version = scribeVersion
	if err := scribeCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
