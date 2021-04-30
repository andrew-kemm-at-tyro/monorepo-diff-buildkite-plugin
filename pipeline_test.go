package main

import (
	"io/ioutil"
	"os"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestMain(m *testing.M) {
	// disable logs in test
	log.SetOutput(ioutil.Discard)

	// set some env variables for using in tests
	os.Setenv("BUILDKITE_COMMIT", "123")
	os.Setenv("BUILDKITE_MESSAGE", "fix: temp file not correctly deleted")
	os.Setenv("BUILDKITE_BRANCH", "go-rewrite")
	os.Setenv("env3", "env-3")
	os.Setenv("env4", "env-4")
	os.Setenv("TEST_MODE", "true")

	run := m.Run()

	os.Exit(run)
}

func mockGeneratePipeline(steps []Step, plugin Plugin) (*os.File, error) {
	mockFile, _ := os.Create("pipeline.txt")
	defer mockFile.Close()

	return mockFile, nil
}

func TestUploadPipelineCallsBuildkiteAgentCommand(t *testing.T) {
	plugin := Plugin{Diff: "echo ./foo-service"}
	cmd, args, err := uploadPipeline(plugin, mockGeneratePipeline)

	assert.Equal(t, "buildkite-agent", cmd)
	assert.Equal(t, []string{"pipeline", "upload", "pipeline.txt"}, args)
	assert.Equal(t, err, nil)
}

func TestUploadPipelineCallsBuildkiteAgentCommandWithInterpolation(t *testing.T) {
	plugin := Plugin{Diff: "echo ./foo-service", Interpolation: true}
	cmd, args, err := uploadPipeline(plugin, mockGeneratePipeline)

	assert.Equal(t, "buildkite-agent", cmd)
	assert.Equal(t, []string{"pipeline", "upload", "pipeline.txt", "--no-interpolation"}, args)
	assert.Equal(t, err, nil)
}

func TestUploadPipelineCancelsIfThereIsNoDiffOutput(t *testing.T) {
	plugin := Plugin{Diff: "echo"}
	cmd, args, err := uploadPipeline(plugin, mockGeneratePipeline)

	assert.Equal(t, "", cmd)
	assert.Equal(t, []string{}, args)
	assert.Equal(t, err, nil)
}

func TestDiff(t *testing.T) {
	want := []string{
		"services/foo/serverless.yml",
		"services/bar/config.yml",
		"ops/bar/config.yml",
		"README.md",
	}

	got, err := diff(`echo services/foo/serverless.yml
services/bar/config.yml

ops/bar/config.yml
README.md`)

	assert.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestPipelinesToTriggerGetsListOfPipelines(t *testing.T) {
	want := []string{"service-1", "service-2", "service-4"}

	watch := []WatchConfig{
		{
			Paths: []string{"watch-path-1"},
			Step:  Step{Trigger: "service-1"},
		},
		{
			Paths: []string{"watch-path-2/", "watch-path-3/", "watch-path-4"},
			Step:  Step{Trigger: "service-2"},
		},
		{
			Paths: []string{"watch-path-5"},
			Step:  Step{Trigger: "service-3"},
		},
		{
			Paths: []string{"watch-path-2"},
			Step:  Step{Trigger: "service-4"},
		},
	}

	changedFiles := []string{
		"watch-path-1/text.txt",
		"watch-path-2/.gitignore",
		"watch-path-2/src/index.go",
		"watch-path-4/test/index_test.go",
	}

	pipelines, err := stepsToTrigger(changedFiles, watch)
	assert.NoError(t, err)
	var got []string

	for _, v := range pipelines {
		got = append(got, v.Trigger)
	}

	assert.Equal(t, want, got)
}

func TestPipelinesStepsToTrigger(t *testing.T) {

	testCases := map[string]struct {
		ChangedFiles []string
		WatchConfigs []WatchConfig
		Expected     []Step
	}{
		"service-1": {
			ChangedFiles: []string{
				"watch-path-1/text.txt",
				"watch-path-2/.gitignore",
			},
			WatchConfigs: []WatchConfig{{
				Paths: []string{"watch-path-1"},
				Step:  Step{Trigger: "service-1"},
			}},
			Expected: []Step{
				{Trigger: "service-1"},
			},
		},
		"service-1-2": {
			ChangedFiles: []string{
				"watch-path-1/text.txt",
				"watch-path-2/.gitignore",
			},
			WatchConfigs: []WatchConfig{
				{
					Paths: []string{"watch-path-1"},
					Step:  Step{Trigger: "service-1"},
				},
				{
					Paths: []string{"watch-path-2"},
					Step:  Step{Trigger: "service-2"},
				},
			},
			Expected: []Step{
				{Trigger: "service-1"},
				{Trigger: "service-2"},
			},
		},
		"extension wildcard": {
			ChangedFiles: []string{
				"text.txt",
				".gitignore",
			},
			WatchConfigs: []WatchConfig{
				{
					Paths: []string{"*.txt"},
					Step:  Step{Trigger: "txt"},
				},
			},
			Expected: []Step{
				{Trigger: "txt"},
			},
		},
		"extension wildcard in subdir": {
			ChangedFiles: []string{
				"docs/text.txt",
			},
			WatchConfigs: []WatchConfig{
				{
					Paths: []string{"docs/*.txt"},
					Step:  Step{Trigger: "txt"},
				},
			},
			Expected: []Step{
				{Trigger: "txt"},
			},
		},
		"directory wildcard": {
			ChangedFiles: []string{
				"docs/text.txt",
			},
			WatchConfigs: []WatchConfig{
				{
					Paths: []string{"**/text.txt"},
					Step:  Step{Trigger: "txt"},
				},
			},
			Expected: []Step{
				{Trigger: "txt"},
			},
		},
		"directory and extension wildcard": {
			ChangedFiles: []string{
				"package/other.txt",
			},
			WatchConfigs: []WatchConfig{
				{
					Paths: []string{"*/*.txt"},
					Step:  Step{Trigger: "txt"},
				},
			},
			Expected: []Step{
				{Trigger: "txt"},
			},
		},
		"double directory and extension wildcard": {
			ChangedFiles: []string{
				"package/docs/other.txt",
			},
			WatchConfigs: []WatchConfig{
				{
					Paths: []string{"**/*.txt"},
					Step:  Step{Trigger: "txt"},
				},
			},
			Expected: []Step{
				{Trigger: "txt"},
			},
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			steps, err := stepsToTrigger(tc.ChangedFiles, tc.WatchConfigs)
			assert.NoError(t, err)
			assert.Equal(t, tc.Expected, steps)
		})
	}
}

func TestPipelinesStepsToTrigger_SetsStepKey(t *testing.T) {
	changedFiles := []string{"service-a.txt"}
	watchConfigs := []WatchConfig{
		{
			Paths: []string{"service-a.txt"},
			Key:   "service-a",
			Step:  Step{Trigger: "service-a-trigger"},
		},
	}
	expectedSteps := []Step{
		Step{
			Trigger: "service-a-trigger",
			Key:     "service-a",
		},
	}

	steps, err := stepsToTrigger(changedFiles, watchConfigs)

	assert.NoError(t, err)
	assert.Equal(t, expectedSteps, steps)
}

func TestPipelinesStepsToTrigger_LetsExistingKeyBe(t *testing.T) {
	changedFiles := []string{"service-a.txt"}
	watchConfigs := []WatchConfig{
		{
			Paths: []string{"service-a.txt"},
			Step: Step{
				Key:     "existing-key",
				Trigger: "service-a-trigger",
			},
		},
	}
	expectedSteps := []Step{
		Step{
			Trigger: "service-a-trigger",
			Key:     "existing-key",
		},
	}

	steps, err := stepsToTrigger(changedFiles, watchConfigs)

	assert.NoError(t, err)
	assert.Equal(t, expectedSteps, steps)
}

// If a steps dependency has changed, it should be triggered.
func TestStepsToTrigger_TriggersAStepWhenDependencyHasTriggered(t *testing.T) {
	changedFiles := []string{"service-a.txt"}
	watchConfigs := []WatchConfig{
		{
			Key:   "service-a-key",
			Paths: []string{"service-a.txt"},
			Step:  Step{Trigger: "step-a-trigger"},
		},
		{
			Key:       "service-b-key",
			Paths:     []string{"service-b.txt"},
			DependsOn: []string{"service-a-key"},
			Step:      Step{Trigger: "step-b-trigger"},
		},
	}
	expectedSteps := []Step{
		{
			Key:     "service-a-key",
			Trigger: "step-a-trigger",
		},
		{
			Key:       "service-b-key",
			Trigger:   "step-b-trigger",
			DependsOn: []string{"service-a-key"},
		},
	}

	steps, err := stepsToTrigger(changedFiles, watchConfigs)

	assert.NoError(t, err)
	assert.Equal(t, expectedSteps, steps)
}

// If a steps dependencies dependency has changed, it should be triggered
// If a steps dependency has changed, it should have the property "depends_on" set to the dependency key
func TestStepsToTrigger_TriggersAStepWhenDependenciesDependencyHasTriggered(t *testing.T) {
	changedFiles := []string{"service-a.txt"}
	watchConfigs := []WatchConfig{
		{
			Key:   "service-a-key",
			Paths: []string{"service-a.txt"},
			Step:  Step{Trigger: "step-a-trigger"},
		},
		{
			Key:       "service-b-key",
			Paths:     []string{"service-b.txt"},
			DependsOn: []string{"service-a-key"},
			Step:      Step{Trigger: "step-b-trigger"},
		},
		{
			Key:       "service-c-key",
			Paths:     []string{"service-c.txt"},
			DependsOn: []string{"service-b-key"},
			Step:      Step{Trigger: "step-c-trigger"},
		},
	}
	expectedSteps := []Step{
		{
			Key:     "service-a-key",
			Trigger: "step-a-trigger",
		},
		{
			Key:       "service-b-key",
			Trigger:   "step-b-trigger",
			DependsOn: []string{"service-a-key"},
		},
		{
			Key:       "service-c-key",
			Trigger:   "step-c-trigger",
			DependsOn: []string{"service-b-key"},
		},
	}

	steps, err := stepsToTrigger(changedFiles, watchConfigs)

	assert.NoError(t, err)
	assert.Equal(t, expectedSteps, steps)
}

// If a steps dependency has _not_ changed, it should not have the property "depends_on"
func TestStepsToTrigger_ShouldNotAddDependsOnIfDependencyHasNotTriggered(t *testing.T) {
	changedFiles := []string{"service-b.txt"}
	watchConfigs := []WatchConfig{
		{
			Key:   "service-a-key",
			Paths: []string{"service-a.txt"},
			Step:  Step{Trigger: "step-a-trigger"},
		},
		{
			Key:       "service-b-key",
			Paths:     []string{"service-b.txt"},
			DependsOn: []string{"service-a-key"},
			Step:      Step{Trigger: "step-b-trigger"},
		},
	}
	expectedSteps := []Step{
		{
			Key:     "service-b-key",
			Trigger: "step-b-trigger",
		},
	}

	steps, err := stepsToTrigger(changedFiles, watchConfigs)

	assert.NoError(t, err)
	assert.Equal(t, expectedSteps, steps)
}

// Given a step has multiple dependencies
// When only a single dependency has changed
// Then only the changed dependency should appear in it's "depends_on" property
func TestStepsToTrigger_ShouldAddToDependsOnOnlyIfDependencyHasTriggered(t *testing.T) {
	changedFiles := []string{"service-a.txt"}
	watchConfigs := []WatchConfig{
		{
			Key:   "service-a-key",
			Paths: []string{"service-a.txt"},
			Step:  Step{Trigger: "step-a-trigger"},
		},
		{
			Key:   "service-b-key",
			Paths: []string{"service-b.txt"},
			Step:  Step{Trigger: "step-b-trigger"},
		},
		{
			Key:       "service-c-key",
			Paths:     []string{"service-c.txt"},
			DependsOn: []string{"service-a-key", "service-b-key"},
			Step:      Step{Trigger: "step-c-trigger"},
		},
	}
	expectedSteps := []Step{
		{
			Key:     "service-a-key",
			Trigger: "step-a-trigger",
		},
		{
			Key:       "service-c-key",
			Trigger:   "step-c-trigger",
			DependsOn: []string{"service-a-key"},
		},
	}

	steps, err := stepsToTrigger(changedFiles, watchConfigs)

	assert.NoError(t, err)
	assert.Equal(t, expectedSteps, steps)
}

// If a step and it's dependency both directly trigger, the step should include it's dependency in DependsOn.
func TestStepsToTrigger_ShouldAddADependsOnIfAStepAndItsDependencyTriggerIndependently(t *testing.T) {
	changedFiles := []string{"service-a.txt", "service-b.txt"}
	watchConfigs := []WatchConfig{
		{
			Key:   "service-a-key",
			Paths: []string{"service-a.txt"},
			Step:  Step{Trigger: "step-a-trigger"},
		},
		{
			Key:       "service-b-key",
			Paths:     []string{"service-b.txt"},
			Step:      Step{Trigger: "step-b-trigger"},
			DependsOn: []string{"service-a-key"},
		},
	}
	expectedSteps := []Step{
		{
			Key:     "service-a-key",
			Trigger: "step-a-trigger",
		},
		{
			Key:       "service-b-key",
			Trigger:   "step-b-trigger",
			DependsOn: []string{"service-a-key"},
		},
	}

	steps, err := stepsToTrigger(changedFiles, watchConfigs)

	assert.NoError(t, err)
	assert.Equal(t, expectedSteps, steps)

}

func TestAnnotateStep_AddsKey(t *testing.T) {
	watchConfig := WatchConfig{
		Paths: []string{"service-a.txt"},
		Key:   "service-a",
		Step:  Step{Trigger: "service-a-trigger"},
	}
	expectedStep := Step{
		Key:     "service-a",
		Trigger: "service-a-trigger",
	}

	step := annotateStep(watchConfig, []string{})

	assert.Equal(t, expectedStep, step)
}

func TestAnnotateStep_AddsDependsOn(t *testing.T) {
	watchConfig := WatchConfig{
		Paths: []string{"service-a.txt"},
		Key:   "service-a",
		Step:  Step{Trigger: "service-a-trigger"},
	}
	expectedStep := Step{
		Key:       "service-a",
		Trigger:   "service-a-trigger",
		DependsOn: []string{"service-b"},
	}

	step := annotateStep(watchConfig, []string{"service-b"})

	assert.Equal(t, expectedStep, step)
}

func TestGeneratePipeline(t *testing.T) {
	steps := []Step{
		{
			Trigger: "foo-service-pipeline",
			Build:   Build{Message: "build message"},
		},
	}

	want :=
		`steps:
- trigger: foo-service-pipeline
  build:
    message: build message
- wait
- command: echo "hello world"
- command: cat ./file.txt`

	plugin := Plugin{
		Wait: true,
		Hooks: []HookConfig{
			{Command: "echo \"hello world\""},
			{Command: "cat ./file.txt"},
		},
	}

	pipeline, err := generatePipeline(steps, plugin)
	defer os.Remove(pipeline.Name())

	if err != nil {
		assert.Equal(t, true, false, err.Error())
	}

	got, _ := ioutil.ReadFile(pipeline.Name())

	assert.Equal(t, want, string(got))
}
