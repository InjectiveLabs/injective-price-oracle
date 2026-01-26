package main

import (
	"fmt"
	"os"

	log "github.com/InjectiveLabs/suplog"
	cli "github.com/jawher/mow.cli"

	"github.com/InjectiveLabs/injective-price-oracle/version"
)

var app = cli.App("injective-price-oracle", "Injective's Oracle with dynamic price feeds.")

var (
	envName        *string
	appLogLevel    *string
	svcWaitTimeout *string
)

func panicIf(err error, msg ...interface{}) {
	if err != nil {
		log.WithError(err).Errorln(msg...)
		panic(err)
	}
}

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
	app.Command("version", "Print the version information and exit.", versionCmd)

	_ = app.Run(os.Args)
}

func versionCmd(c *cli.Cmd) {
	c.Action = func() {
		fmt.Println(version.Version())
	}
}
