package testcases

import "regexp"

// E2E-CLI-009 - kics scan with no-progress flag
// should perform a scan without showing progress bar in the CLI
// test to be removed in the future due to deprecation of --no-progress
func init() { //nolint
	testSample := TestCase{
		Name: "should hide the progress bar in the CLI [E2E-CLI-009]",
		Args: args{
			Args: []cmdArgs{
				[]string{"scan", "-p", "/path/e2e/fixtures/samples/positive.dockerfile"},
			},
		},
		WantStatus: []int{50},
		Validation: func(outputText string) bool {
			getProgressRegex := "Executing queries:"
			match, _ := regexp.MatchString(getProgressRegex, outputText)
			// if found -> the the test was successful
			return match
		},
	}

	Tests = append(Tests, testSample)
}
