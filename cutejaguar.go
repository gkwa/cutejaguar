package cutejaguar

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/smithy-go"
	"github.com/jessevdk/go-flags"
	"github.com/taylormonacelli/lemondrop"
)

// Result represents the result of listing launch templates.
type Result struct {
	Region       lemondrop.RegionComponents
	TemplateName string
	ConsoleURL   string // URL pointing to the AWS Management Console for the launch template in the region
}

// AuthFailure represents authentication failure details.
type AuthFailure struct {
	Region  lemondrop.RegionComponents
	Code    string
	Message string
}

var opts struct {
	LogFormat    string `long:"log-format" choice:"text" choice:"json" default:"text" required:"false"`
	Verbose      []bool `short:"v" long:"verbose" description:"Show verbose debug information, each -v bumps log level"`
	NoAuthErrors bool   `long:"no-autherror" description:"Do not show authentication errors"`
	logLevel     slog.Level
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
		return fmt.Errorf("failed to get region details: %w", err)
	}

	var errgrp errgroup.Group
	semaphore := make(chan struct{}, 10) // Semaphore to limit routines

	// Use a mutex to safely append to the results slice and failures slice
	var mu sync.Mutex
	var results []Result
	var authFailures []AuthFailure
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
				var apiErr smithy.APIError
				if errors.As(err, &apiErr) {
					if apiErr.ErrorCode() == "AuthFailure" {
						mu.Lock()
						authFailures = append(authFailures, AuthFailure{
							Region:  region,
							Code:    apiErr.ErrorCode(),
							Message: apiErr.ErrorMessage(),
						})
						mu.Unlock()
					}
				}
				return err
			}

			mu.Lock()
			for _, template := range templates {
				results = append(results, Result{
					Region:       region,
					TemplateName: template,
					ConsoleURL: fmt.Sprintf("https://%s.console.aws.amazon.com/ec2/home?region=%s#LaunchTemplates:",
						region.RegionCode, region.RegionCode),
				})
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
		return results[i].Region.RegionCode < results[j].Region.RegionCode
	})

	// Print launch templates
	for _, result := range results {
		fmt.Printf("%s %s %s\n", result.Region.RegionCode, result.TemplateName, result.ConsoleURL)
	}

	if !opts.NoAuthErrors {
		for _, failure := range authFailures {
			slog.Error("auth Failure", "region", failure.Region.RegionCode, "code", failure.Code, "message", failure.Message)
		}
	}

	slog.Info("templates", "count", len(results))

	return nil
}

func listLaunchTemplates(region lemondrop.RegionComponents) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel() // This is important to release resources associated with the context

	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region.RegionCode))
	if err != nil {
		return nil, err
	}

	client := ec2.NewFromConfig(cfg)

	// List launch templates by name
	listTemplatesOutput, err := client.DescribeLaunchTemplates(ctx, &ec2.DescribeLaunchTemplatesInput{})
	if err != nil {
		return []string{}, err
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
