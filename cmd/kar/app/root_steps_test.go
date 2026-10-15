/* jscpd:ignore-start */
/*
Copyright © 2025

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
/* jscpd:ignore-end */

package app_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/cucumber/godog"
	"github.com/electrocucaracha/kubevirt-actions-runner/cmd/kar/app"
	runner "github.com/electrocucaracha/kubevirt-actions-runner/internal"
	"github.com/spf13/cobra"
)

var errExpectedFailure = errors.New("failure")

var (
	errRootCommandAssertion     = errors.New("root command assertion failed")
	errCreateResourcesNotCalled = errors.New("CreateResources was not called")
	errWaitForVMINotCalled      = errors.New("WaitForVirtualMachineInstance was not called")
	errDeleteResourcesNotCalled = errors.New("DeleteResources was not called")
	errExpectedCommandFailure   = errors.New("expected command failure")
	errDeleteCallCount          = errors.New("unexpected number of DeleteResources calls")
	errCleanupContextCancelled  = errors.New("DeleteResources received a cancelled context")
	errCleanupContextDeadline   = errors.New("DeleteResources received a context without a deadline")
	errCleanupContextUnbounded  = errors.New("DeleteResources received a context with a deadline")
)

// cleanupTimeout is the cleanup window passed to the root command in tests.
const cleanupTimeout = 5 * time.Minute

// mock implements the runner.Runner interface for exercising the root command.
type mock struct {
	createErr    error
	deleteErr    error
	waitErr      error
	createCalled bool
	waitCalled   bool
	deleteCalls  int
	deleteCtxErr error
	deleteHasDL  bool
	onWait       func()
	vmTemplate   string
	vmTemplateNS string
	runnerName   string
	jitConfig    string
}

func (m *mock) CreateResources(
	_ context.Context,
	vmTemplate,
	vmTemplateNamespace,
	runnerName,
	jitConfig string,
) error {
	m.vmTemplate = vmTemplate
	m.vmTemplateNS = vmTemplateNamespace
	m.runnerName = runnerName
	m.jitConfig = jitConfig

	m.createCalled = true

	return m.createErr
}

func (m *mock) WaitForVirtualMachineInstance(_ context.Context) error {
	m.waitCalled = true

	if m.onWait != nil {
		m.onWait()
	}

	return m.waitErr
}

func (m *mock) DeleteResources(ctx context.Context) error {
	m.deleteCalls++
	m.deleteCtxErr = ctx.Err()
	_, m.deleteHasDL = ctx.Deadline()

	return m.deleteErr
}

type rootCmdCtx struct {
	runner *mock
	cmd    *cobra.Command
	result error
}

func (rc *rootCmdCtx) reset(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
	rc.runner = new(mock)
	rc.cmd = newTestRootCommand(context.TODO(), rc.runner)
	rc.result = nil

	return ctx, nil
}

func newTestRootCommand(ctx context.Context, r runner.Runner) *cobra.Command {
	var opts app.Opts

	opts.CleanupTimeout = cleanupTimeout

	return app.NewRootCommand(ctx, r, opts)
}

func (rc *rootCmdCtx) aMockRunner() error {
	return nil
}

func (rc *rootCmdCtx) aMockRunnerWithoutCleanupTimeout() error {
	var opts app.Opts

	rc.cmd = app.NewRootCommand(context.TODO(), rc.runner, opts)

	return nil
}

// aMockRunnerInterruptedBySignal simulates a SIGTERM arriving while the
// runner waits: the parent context is cancelled and the wait returns.
func (rc *rootCmdCtx) aMockRunnerInterruptedBySignal() error {
	ctx, cancel := context.WithCancel(context.Background())

	rc.runner.onWait = func() {
		cancel()

		rc.runner.waitErr = ctx.Err()
	}
	rc.cmd = newTestRootCommand(ctx, rc.runner)

	return nil
}

//nolint:cyclop,funlen // Mirrors the assertions of the original DescribeTable entries.
func (rc *rootCmdCtx) executeRootCommand(flag, value, failure string) error {
	var args []string
	if flag != "" {
		args = []string{flag, value}
	}

	rc.cmd.SetArgs(args)

	switch failure {
	case "create":
		rc.runner.createErr = errExpectedFailure
	case "delete":
		rc.runner.deleteErr = errExpectedFailure
	case "wait":
		rc.runner.waitErr = errExpectedFailure
	case "runner failed":
		rc.runner.waitErr = runner.ErrRunnerFailed
	}

	rc.result = rc.cmd.Execute()

	switch flag {
	case "-c":
		if rc.runner.jitConfig != value {
			return fmt.Errorf("%w: JIT config mismatch: want %q, got %q", errRootCommandAssertion, value, rc.runner.jitConfig)
		}
	case "-r":
		if rc.runner.runnerName != value {
			return fmt.Errorf("%w: runner name mismatch: want %q, got %q", errRootCommandAssertion, value, rc.runner.runnerName)
		}
	case "-t":
		if rc.runner.vmTemplate != value {
			return fmt.Errorf("%w: VM template mismatch: want %q, got %q", errRootCommandAssertion, value, rc.runner.vmTemplate)
		}
	case "-n":
		if rc.runner.vmTemplateNS != value {
			return fmt.Errorf(
				"%w: VM template namespace mismatch: want %q, got %q",
				errRootCommandAssertion,
				value,
				rc.runner.vmTemplateNS,
			)
		}
	}

	if flag != "-n" && rc.runner.vmTemplateNS != "default" {
		return fmt.Errorf(
			"%w: expected default VM template namespace, got %q",
			errRootCommandAssertion,
			rc.runner.vmTemplateNS,
		)
	}

	if !rc.runner.createCalled {
		return errCreateResourcesNotCalled
	}

	if failure == "create" {
		return nil
	}

	if !rc.runner.waitCalled {
		return errWaitForVMINotCalled
	}

	if rc.runner.deleteCalls == 0 {
		return errDeleteResourcesNotCalled
	}

	return nil
}

func (rc *rootCmdCtx) deleteResourcesShouldBeCalled(count int) error {
	if rc.runner.deleteCalls != count {
		return fmt.Errorf("%w: want %d, got %d", errDeleteCallCount, count, rc.runner.deleteCalls)
	}

	return nil
}

func (rc *rootCmdCtx) cleanupContextShouldBeLive() error {
	if rc.runner.deleteCtxErr != nil {
		return fmt.Errorf("%w: %w", errCleanupContextCancelled, rc.runner.deleteCtxErr)
	}

	if !rc.runner.deleteHasDL {
		return errCleanupContextDeadline
	}

	return nil
}

func (rc *rootCmdCtx) commandExecutionShould(outcome string) error {
	switch outcome {
	case "succeed":
		if rc.result != nil {
			return fmt.Errorf("expected command to succeed, but it failed: %w", rc.result)
		}
	case "fail":
		if rc.result == nil {
			return fmt.Errorf("%w, but it succeeded", errExpectedCommandFailure)
		}
	}

	return nil
}

func (rc *rootCmdCtx) cleanupContextShouldBeUnbounded() error {
	if rc.runner.deleteHasDL {
		return errCleanupContextUnbounded
	}

	return nil
}

func InitializeScenario(scenarioCtx *godog.ScenarioContext) {
	rootCtx := new(rootCmdCtx)

	scenarioCtx.Before(rootCtx.reset)

	scenarioCtx.Step(`^a mock runner$`, rootCtx.aMockRunner)
	scenarioCtx.Step(`^a mock runner whose wait is interrupted by a termination signal$`,
		rootCtx.aMockRunnerInterruptedBySignal)
	scenarioCtx.Step(`^a mock runner without a cleanup timeout$`, rootCtx.aMockRunnerWithoutCleanupTimeout)
	scenarioCtx.Step(`^the root command is executed with flag "([^"]*)" value "([^"]*)" and induced failure "([^"]*)"$`,
		rootCtx.executeRootCommand)
	scenarioCtx.Step(`^the command execution should "([^"]*)"$`, rootCtx.commandExecutionShould)
	scenarioCtx.Step(`^DeleteResources should be called (\d+) times?$`, rootCtx.deleteResourcesShouldBeCalled)
	scenarioCtx.Step(`^DeleteResources should receive a live context with a deadline$`,
		rootCtx.cleanupContextShouldBeLive)
	scenarioCtx.Step(`^DeleteResources should receive a context without a deadline$`,
		rootCtx.cleanupContextShouldBeUnbounded)
}

func TestFeatures(t *testing.T) {
	t.Parallel()

	options := new(godog.Options)
	options.Format = "pretty"
	options.Paths = []string{"features"}
	options.TestingT = t

	suite := new(godog.TestSuite)
	suite.ScenarioInitializer = InitializeScenario
	suite.Options = options

	if suite.Run() != 0 {
		t.Fatal("non-zero status returned, failed to run feature tests")
	}
}
