package cutejaguar

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"golang.org/x/sync/errgroup"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/jessevdk/go-flags"
	"github.com/taylormonacelli/lemondrop"
)

var opts struct {
	LogFormat string `long:"log-format" choice:"text" choice:"json" default:"text" required:"false"`
	Verbose   []bool `short:"v" long:"verbose" description:"Show verbose debug information, each -v bumps log level"`
	logLevel  slog.Level
}

func Execute() int {
	if err := parseFlags(); err != nil {
		return 1
	}

	if err := setLogLevel(); err != nil {
		return 1
	}

	if err := setupLogger(); err != nil {
		return 1
	}

	if err := run(); err != nil {
		slog.Error("run failed", "error", err)
		return 1
	}

	return 0
}

func parseFlags() error {
	_, err := flags.Parse(&opts)
	return err
}

func run() error {
	regions, err := lemondrop.GetRegionDetails()
	if err != nil {
		panic(err)
	}

	var errgrp errgroup.Group
	semaphore := make(chan struct{}, 10) // Semaphore to limit routines

	// Use a mutex to safely append to the results slice
	var mu sync.Mutex
	var results []string

	for _, region := range regions {
		region := region // capture range variable
		slog.Debug("checking", "region", region.RegionCode)
		semaphore <- struct{}{} // acquire semaphore
		errgrp.Go(func() error {
			defer func() {
				<-semaphore // release semaphore
			}()
			templates, err := listLaunchTemplates(region.RegionCode)
			if err != nil {
				// us-gov-east-1
				slog.Error("Error in region", "region", region, "error", err)
				return err
			}
			mu.Lock()
			results = append(results, templates...)
			mu.Unlock()
			return nil
		})
	}

	slog.Debug("we get here")

	if err := errgrp.Wait(); err != nil {
		return err
	}

	fmt.Println(results)

	return nil
}

func listLaunchTemplates(region string) ([]string, error) {
	cfg, err := config.LoadDefaultConfig(context.TODO(), config.WithRegion(region))
	if err != nil {
		return nil, err
	}

	client := ec2.NewFromConfig(cfg)

	// List launch templates by name
	listTemplatesOutput, err := client.DescribeLaunchTemplates(context.TODO(), &ec2.DescribeLaunchTemplatesInput{})
	if err != nil {
		return nil, err
	}

	var templates []string
	for _, template := range listTemplatesOutput.LaunchTemplates {
		templates = append(templates, fmt.Sprintf("%s %s", region, *template.LaunchTemplateName))
	}

	return templates, nil
}
