package cutejaguar

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"sync"

	"golang.org/x/sync/errgroup"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/smithy-go"
	"github.com/jessevdk/go-flags"
	"github.com/taylormonacelli/lemondrop"
)

// Result represents the result of listing launch templates.
type Result struct {
	Region       string
	TemplateName string
}

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
	var results []Result
	done := make(chan struct{}) // Channel to signal completion

	for _, region := range regions {
		region := region // capture range variable
		slog.Debug("checking", "region", region.RegionCode)
		semaphore <- struct{}{} // acquire semaphore
		errgrp.Go(func() error {
			defer func() {
				<-semaphore // release semaphore
			}()
			templates, err := listLaunchTemplates(region)
			if err != nil {
				// Check if the error is an API error
				// us-gov-east-1
				var apiErr smithy.APIError
				if errors.As(err, &apiErr) {
					if apiErr.ErrorCode() == "AuthFailure" {
						slog.Error("apierr", "code", apiErr.ErrorCode(), "message", apiErr.ErrorMessage())
					}
				}
			}

			mu.Lock()
			for _, template := range templates {
				results = append(results, Result{Region: region.RegionCode, TemplateName: template})
			}
			mu.Unlock()
			return nil
		})
	}

	slog.Debug("we get here")

	go func() {
		if err := errgrp.Wait(); err != nil {
			slog.Error("Error in errgroup", "error", err)
		}
		close(done) // Signal completion
	}()

	<-done // Wait for completion before proceeding

	// Sort results by region
	sort.Slice(results, func(i, j int) bool {
		return results[i].Region < results[j].Region
	})

	// Format and print the sorted results
	for _, result := range results {
		fmt.Printf("%s %s\n", result.Region, result.TemplateName)
	}

	slog.Info("templates", "count", len(results))

	return nil
}

func listLaunchTemplates(region lemondrop.RegionComponents) ([]string, error) {
	cfg, err := config.LoadDefaultConfig(context.TODO(), config.WithRegion(region.RegionCode))
	if err != nil {
		return nil, err
	}

	client := ec2.NewFromConfig(cfg)

	// List launch templates by name
	listTemplatesOutput, err := client.DescribeLaunchTemplates(context.TODO(), &ec2.DescribeLaunchTemplatesInput{})
	if err != nil {
		// Check if the error is an API error
		// us-gov-east-1
		var apiErr smithy.APIError
		if errors.As(err, &apiErr) {
			if apiErr.ErrorCode() == "AuthFailure" {
				slog.Error("apierr", "region", region, "code", apiErr.ErrorCode(), "message", apiErr.ErrorMessage())
			}
		}
	}

	if listTemplatesOutput == nil {
		return []string{}, nil
	}

	var templates []string
	for _, template := range listTemplatesOutput.LaunchTemplates {
		templates = append(templates, *template.LaunchTemplateName)
	}

	return templates, nil
}
