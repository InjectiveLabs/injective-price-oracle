package main

import (
	"context"
	"io/ioutil"

	cli "github.com/jawher/mow.cli"
	"github.com/xlab/closer"
	log "github.com/xlab/suplog"

	"github.com/InjectiveLabs/injective-price-oracle/oracle"
)

// probeCmd action validates target TOML file spec and runs it once, printing the result.
//
// $ injective-price-oracle probe <FILE>
func probeCmd(cmd *cli.Cmd) {
	tomlSource := cmd.StringArg("FILE", "", "Path to target TOML file with pipeline spec")

	cmd.Action = func() {
		// ensure a clean exit
		defer closer.Close()

		cfgBody, err := ioutil.ReadFile(*tomlSource)
		if err != nil {
			log.WithField("file", *tomlSource).WithError(err).Fatalln("failed to read dynamic feed config")
			return
		}

		feedCfg, err := oracle.ParseDynamicFeedConfig(cfgBody)
		if err != nil {
			log.WithError(err).WithFields(log.Fields{
				"file": *tomlSource,
			}).Errorln("failed to parse dynamic feed config")
			return
		}

		pricePuller, err := oracle.NewDynamicPriceFeed(feedCfg)
		if err != nil {
			log.WithError(err).Fatalln("failed to init new dynamic price feed")
			return
		}

		pullerLogger := log.WithFields(log.Fields{
			"provider_name": pricePuller.ProviderName(),
			"symbol":        pricePuller.Symbol(),
		})

		answer, err := pricePuller.PullPrice(context.Background())
		if err != nil {
			pullerLogger.WithError(err).Errorln("failed to pull price")
			return
		}

		log.Infof("Answer: %s", answer.String())
	}
}
