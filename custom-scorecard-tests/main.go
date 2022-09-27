/*
Copyright 2022 The VolSync authors.

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published
by the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"

	scapiv1alpha3 "github.com/operator-framework/api/pkg/apis/scorecard/v1alpha3"
	apimanifests "github.com/operator-framework/api/pkg/manifests"
)

// This is using the sample image here as a starting point:
// https://github.com/operator-framework/operator-sdk/tree/master/images/custom-scorecard-tests
//
// The intention here is to create a custom-scorecard-image that can be used for
// scorecard tests and run our existing e2e tests against a system that has
// the operator installed.

var version = "0.0.0"

const PodBundleRoot = "/bundle"

const (
	DeployPrereqsName = "deploy-prereqs"
)

var validTests = []string{
	DeployPrereqsName,
}

func main() {
	entrypoint := os.Args[1:]
	if len(entrypoint) == 0 {
		log.Fatal("Test name argument is required")
	}

	// Read the pod's untar'd bundle from a well-known path.
	cfg, err := apimanifests.GetBundleFromDir(PodBundleRoot)
	if err != nil {
		log.Fatal(err.Error())
	}

	// Get the list of available ansible e2e tests
	ansiblePlaybookTests, err := getAvailableAnsiblePlaybookTests()
	if err != nil {
		log.Fatal(err.Error())
	}

	validTests = append(validTests, ansiblePlaybookTests...)

	var result scapiv1alpha3.TestStatus

	// Names of the custom tests which would be passed in the
	// `operator-sdk` command.
	switch entrypoint[0] {
	case DeployPrereqsName:
		result = DeployPrereqs(cfg)
	default:
		// Assume it's an ansible playbook test at this point
		if isAnsiblePlaybookTest(ansiblePlaybookTests, entrypoint[0]) {
			result = testAnsiblePlaybook(entrypoint[0])
		} else {
			result = printValidTests()
		}
	}

	// Convert scapiv1alpha3.TestResult to json.
	prettyJSON, err := json.MarshalIndent(result, "", "    ")
	if err != nil {
		log.Fatal("Failed to generate json", err)
	}
	fmt.Printf("%s\n", string(prettyJSON))
}

// printValidTests will print out full list of test names to give a hint to the end user on what the valid tests are.
func printValidTests() scapiv1alpha3.TestStatus {
	r := scapiv1alpha3.TestResult{}
	r.State = scapiv1alpha3.FailState
	r.Errors = make([]string, 0)
	r.Suggestions = make([]string, 0)

	msg := "Valid tests for this image include: \n"

	for _, testName := range validTests {
		msg += fmt.Sprintf("- %s\n", testName)
	}

	r.Errors = append(r.Errors, msg)

	return wrapResult(r)
}

func getAvailableAnsiblePlaybookTests() ([]string, error) {
	ansiblePlaybookTests := []string{}

	homeDir := os.Getenv("HOME")
	testE2EDir := homeDir + "/test-e2e" // e2e tests should be here

	files, err := os.ReadDir(testE2EDir)
	if err != nil {
		return nil, err
	}
	for _, file := range files {
		if strings.HasPrefix(file.Name(), "test_") && strings.HasSuffix(file.Name(), ".yml") {
			ansiblePlaybookTests = append(ansiblePlaybookTests, file.Name())
		}
	}

	return ansiblePlaybookTests, nil
}

func isAnsiblePlaybookTest(availableAnsiblePlaybookTests []string, testName string) bool {
	for _, ansiblePlaybookTest := range availableAnsiblePlaybookTests {
		if testName == ansiblePlaybookTest {
			return true
		}
	}
	return false
}

// Define any operator specific custom tests here.

func DeployPrereqs(bundle *apimanifests.Bundle) scapiv1alpha3.TestStatus {
	r := scapiv1alpha3.TestResult{}
	r.Name = DeployPrereqsName
	r.State = scapiv1alpha3.PassState
	r.Errors = make([]string, 0)
	r.Suggestions = make([]string, 0)

	// helm writes to $HOME so set it to /tmp before running this step
	output, err := shellout("export HOME=/tmp && ./run-minio.sh")

	if err != nil {
		r.State = scapiv1alpha3.FailState
		r.Log = "==FAILED==\n" + output
	} else {
		r.State = scapiv1alpha3.PassState
		r.Log = "==PASSED==\n" + output
	}

	return wrapResult(r)
}

func testAnsiblePlaybook(ansiblePlaybookTestName string) scapiv1alpha3.TestStatus {
	r := scapiv1alpha3.TestResult{}
	r.Name = ansiblePlaybookTestName
	r.State = scapiv1alpha3.PassState
	r.Errors = make([]string, 0)
	r.Suggestions = make([]string, 0)

	output, err := shellout(fmt.Sprintf("cd test-e2e && pipenv run ansible-playbook %s", ansiblePlaybookTestName))

	if err != nil {
		r.State = scapiv1alpha3.FailState
		r.Log = "==FAILED==\n" + output

		dumpLogsOutput, dumpLogsErr := shellout("cd test-e2e && pipenv run ansible-playbook dump_logs.yml")
		if dumpLogsErr != nil {
			r.Log = r.Log + "\n\nError dumping logs: \n" + dumpLogsErr.Error()
		} else {
			r.Log = r.Log + "\n\n==DUMPLOGS==\n" + dumpLogsOutput
		}
	} else {
		r.State = scapiv1alpha3.PassState
		r.Log = "==PASSED==\n" + output
	}

	return wrapResult(r)
}

func wrapResult(r scapiv1alpha3.TestResult) scapiv1alpha3.TestStatus {
	r.Log = fmt.Sprintf("volsync custom scorecard tests build version: %s\n\n", version) + r.Log

	return scapiv1alpha3.TestStatus{
		Results: []scapiv1alpha3.TestResult{r},
	}
}

func shellout(command string) (output string, err error) {
	var stdOutAndErr bytes.Buffer

	cmd := exec.Command("bash", "-c", command)
	cmd.Env = append(cmd.Env, os.Environ()...)
	cmd.Stdout = &stdOutAndErr
	cmd.Stderr = &stdOutAndErr
	err = cmd.Run()
	line := "---------------------------------------------------\n"
	output = line + "COMMAND: " + command + "\n" + "OUTPUT: \n" + stdOutAndErr.String()

	return output, err
}
