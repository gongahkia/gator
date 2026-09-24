package main

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"

	"github.com/gongahkia/gator/internal/delivery"
	"github.com/gongahkia/gator/internal/journal"
)

type workRetryOptions struct {
	workID     string
	deliveryID string
	json       bool
}

func retryWorkDelivery(arguments []string, out io.Writer) error {
	options, err := parseWorkRetryOptions(arguments)
	if err != nil {
		return err
	}
	stateDir, err := journal.ResolveStateDir(os.Getenv("GATOR_STATE_DIR"))
	if err != nil {
		return err
	}
	store, err := delivery.Open(stateDir)
	if err != nil {
		return err
	}
	record, retryErr := store.Retry(context.Background(), options.workID, options.deliveryID)
	return writeDeliveryResult(out, record, retryErr, options.json)
}

func parseWorkRetryOptions(arguments []string) (workRetryOptions, error) {
	options := workRetryOptions{}
	for index := 0; index < len(arguments); index++ {
		argument := arguments[index]
		switch {
		case argument == "--json":
			options.json = true
		case argument == "--delivery":
			if index+1 >= len(arguments) {
				return workRetryOptions{}, errors.New("--delivery requires a delivery ID")
			}
			index++
			options.deliveryID = strings.TrimSpace(arguments[index])
		case strings.HasPrefix(argument, "--delivery="):
			options.deliveryID = strings.TrimSpace(strings.TrimPrefix(argument, "--delivery="))
		case strings.HasPrefix(argument, "-"):
			return workRetryOptions{}, errors.New("usage: gator work retry WORK_ID [--delivery DELIVERY_ID] [--json]")
		case options.workID == "":
			options.workID = argument
		default:
			return workRetryOptions{}, errors.New("usage: gator work retry WORK_ID [--delivery DELIVERY_ID] [--json]")
		}
	}
	if options.workID == "" || options.deliveryID == "" && strings.ContainsAny(options.workID, "/\\") || options.deliveryID != "" && strings.ContainsAny(options.deliveryID, "/\\") {
		return workRetryOptions{}, errors.New("usage: gator work retry WORK_ID [--delivery DELIVERY_ID] [--json]")
	}
	return options, nil
}
