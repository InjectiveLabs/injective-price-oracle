package main

import (
	"fmt"
	"os"

	cli "github.com/jawher/mow.cli"
	log "github.com/xlab/suplog"

	"github.com/InjectiveLabs/injective-price-oracle/version"
)

var app = cli.App("injective-price-oracle", "Injective's Oracle with dynamic price feeds.")

var (
	envName        *string
	appLogLevel    *string
	svcWaitTimeout *string
)

func main() {
	readEnv()
	initGlobalOptions(
		&envName,
		&appLogLevel,
		&svcWaitTimeout,
	)

	app.Before = func() {
		log.DefaultLogger.SetLevel(logLevel(*appLogLevel))
	}

	app.Command("start", "Starts the oracle main loop.", oracleCmd)
	app.Command("probe", "Validates target TOML file spec and runs it once, printing the result.", probeCmd)
	app.Command("version", "Print the version information and exit.", versionCmd)

	_ = app.Run(os.Args)
}

func versionCmd(c *cli.Cmd) {
	c.Action = func() {
		fmt.Println(version.Version())
	}
}
