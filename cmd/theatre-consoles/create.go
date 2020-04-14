package main

import (
	"context"

	kitlog "github.com/go-kit/kit/log"
	"github.com/gocardless/theatre/pkg/workloads/console/runner"
	consoleRunner "github.com/gocardless/theatre/pkg/workloads/console/runner"
)

var (
	createTimeout = create.Flag("timeout", "Timeout for the new console").Duration()
	createReason  = create.Flag("reason", "Reason for creating console").String()
	createCommand = create.Arg("command", "Command to run in console").Strings()
)

// Create attempts to create a console in the given in the given namespace after finding the a template using selectors.
func Create(ctx context.Context, logger kitlog.Logger, runner *runner.Runner, namespace string) error {
	var err error

	// Create and attach to the console
	tpl, err := runner.FindTemplateBySelector(namespace, *cliSelector)
	if err != nil {
		return err
	}

	opt := consoleRunner.Options{Cmd: *createCommand, Timeout: int(createTimeout.Seconds()), Reason: *createReason}
	csl, err := runner.Create(tpl.Namespace, *tpl, opt)
	if err != nil {
		return nil
	}

	csl, err = runner.WaitUntilReady(ctx, *csl)
	if err != nil {
		return nil
	}

	pod, err := runner.GetAttachablePod(csl)
	if err != nil {
		return nil
	}

	logger.Log("pod", pod.Name, "msg", "console pod created")

	return err
}
